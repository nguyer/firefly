// Copyright © 2021 Kaleido, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package broadcast

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/kaleido-io/firefly/internal/batch"
	"github.com/kaleido-io/firefly/internal/blockchain"
	"github.com/kaleido-io/firefly/internal/config"
	"github.com/kaleido-io/firefly/internal/database"
	"github.com/kaleido-io/firefly/internal/fftypes"
	"github.com/kaleido-io/firefly/internal/i18n"
	"github.com/kaleido-io/firefly/internal/publicstorage"
)

type BroadcastManager interface {
	BroadcastMessage(ctx context.Context, msg *fftypes.Message) error
	Start() error
	WaitStop()
}

type broadcastManager struct {
	ctx           context.Context
	database      database.Plugin
	blockchain    blockchain.Plugin
	publicstorage publicstorage.Plugin
	batch         batch.BatchManager
}

func NewBroadcastManager(ctx context.Context, di database.Plugin, bi blockchain.Plugin, pi publicstorage.Plugin, ba batch.BatchManager) (BroadcastManager, error) {
	if di == nil || bi == nil || ba == nil || pi == nil {
		return nil, i18n.NewError(ctx, i18n.MsgInitializationNilDepError)
	}
	bm := &broadcastManager{
		ctx:           ctx,
		database:      di,
		blockchain:    bi,
		publicstorage: pi,
		batch:         ba,
	}
	bo := batch.BatchOptions{
		BatchMaxSize:   config.GetUint(config.BroadcastBatchSize),
		BatchTimeout:   config.GetDuration(config.BroadcastBatchTimeout),
		DisposeTimeout: config.GetDuration(config.BroadcastBatchAgentTimeout),
	}
	ba.RegisterDispatcher(fftypes.MessageTypeBroadcast, bm.dispatchBatch, bo)
	ba.RegisterDispatcher(fftypes.MessageTypeDefinition, bm.dispatchBatch, bo)
	return bm, nil
}

func (bm *broadcastManager) dispatchBatch(ctx context.Context, batch *fftypes.Batch) error {

	// Serialize the full payload, which has already been sealed for us by the BatchManager
	payload, err := json.Marshal(batch)
	if err != nil {
		return i18n.WrapError(ctx, err, i18n.MsgSerializationFailed)
	}

	// Write it to IPFS to get a payload reference hash (might not be the sha256 data hash).
	// The payload ref will be persisted back to the batch, as well as being used in the TX
	var publicstorageID string
	batch.PayloadRef, publicstorageID, err = bm.publicstorage.PublishData(ctx, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	return bm.database.RunAsGroup(ctx, func(ctx context.Context) error {
		return bm.submitTXAndUpdateDB(ctx, batch, batch.PayloadRef, publicstorageID)
	})
}

func (bm *broadcastManager) submitTXAndUpdateDB(ctx context.Context, batch *fftypes.Batch, payloadRef *fftypes.Bytes32, publicstorageID string) error {
	// Write the transation to our DB, to collect transaction submission updates
	tx := &fftypes.Transaction{
		ID: batch.Payload.TX.ID,
		Subject: fftypes.TransactionSubject{
			Type:      fftypes.TransactionTypePin,
			Namespace: batch.Namespace,
			Author:    batch.Author,
			Batch:     batch.ID,
		},
		Created: fftypes.Now(),
		Status:  fftypes.TransactionStatusPending,
	}
	tx.Hash = tx.Subject.Hash()
	err := bm.database.UpsertTransaction(ctx, tx, false /* should be new, or idempotent replay */)
	if err != nil {
		return err
	}

	// Update the batch to store the payloadRef
	err = bm.database.UpdateBatch(ctx, batch.ID, database.BatchQueryFactory.NewUpdate(ctx).Set("payloadref", payloadRef))
	if err != nil {
		return err
	}

	// Write the batch pin to the blockchain
	blockchainTrackingID, err := bm.blockchain.SubmitBroadcastBatch(ctx, batch.Author, &blockchain.BroadcastBatch{
		TransactionID:  batch.Payload.TX.ID,
		BatchID:        batch.ID,
		BatchPaylodRef: batch.PayloadRef,
	})
	if err != nil {
		return err
	}

	// Store the operations for each message
	for _, msg := range batch.Payload.Messages {

		// The pending blockchain transaction
		op := fftypes.NewMessageOp(
			bm.blockchain,
			blockchainTrackingID,
			msg,
			fftypes.OpTypeBlockchainBatchPin,
			fftypes.OpStatusPending,
			"")
		if err := bm.database.UpsertOperation(ctx, op); err != nil {
			return err
		}

		// The completed PublicStorage upload
		op = fftypes.NewMessageOp(
			bm.publicstorage,
			publicstorageID,
			msg,
			fftypes.OpTypePublicStorageBatchBroadcast,
			fftypes.OpStatusSucceeded, // Note we performed the action synchronously above
			"")
		if err := bm.database.UpsertOperation(ctx, op); err != nil {
			return err
		}
	}

	return nil
}

func (bm *broadcastManager) BroadcastMessage(ctx context.Context, msg *fftypes.Message) (err error) {

	// Seal the message
	if err = msg.Seal(ctx); err != nil {
		return err
	}

	// Store the message - this asynchronously triggers the next step in process
	return bm.database.UpsertMessage(ctx, msg, false /* should be new, or idempotent replay */)
}

func (bm *broadcastManager) Start() error {
	return nil
}

func (bm *broadcastManager) WaitStop() {
	// No go routines
}