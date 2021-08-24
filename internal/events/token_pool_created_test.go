// Copyright © 2021 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
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

package events

import (
	"fmt"
	"testing"

	"github.com/hyperledger-labs/firefly/mocks/databasemocks"
	"github.com/hyperledger-labs/firefly/mocks/tokenmocks"
	"github.com/hyperledger-labs/firefly/pkg/database"
	"github.com/hyperledger-labs/firefly/pkg/fftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestTokenPoolCreatedSuccess(t *testing.T) {
	em, cancel := newTestEventManager(t)
	defer cancel()
	mdi := em.database.(*databasemocks.Plugin)
	mti := &tokenmocks.Plugin{}

	pool := &fftypes.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "test-ns",
		Name:      "my-pool",
	}

	mdi.On("UpsertTokenPool", em.ctx, pool, false).Return(nil)
	mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *fftypes.Event) bool {
		return ev.Type == fftypes.EventTypePoolConfirmed && ev.Reference == pool.ID && ev.Namespace == pool.Namespace
	})).Return(nil)

	info := fftypes.JSONObject{"some": "info"}
	err := em.TokenPoolCreated(mti, pool, "0x12345", info)
	assert.NoError(t, err)
	mdi.AssertExpectations(t)
}

func TestTokenPoolBadNamespace(t *testing.T) {
	em, cancel := newTestEventManager(t)
	defer cancel()
	mdi := em.database.(*databasemocks.Plugin)
	mti := &tokenmocks.Plugin{}

	pool := &fftypes.TokenPool{}

	mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *fftypes.Event) bool {
		return ev.Type == fftypes.EventTypePoolRejected && ev.Reference == pool.ID && ev.Namespace == pool.Namespace
	})).Return(nil)

	info := fftypes.JSONObject{"some": "info"}
	err := em.TokenPoolCreated(mti, pool, "0x12345", info)
	assert.NoError(t, err)
	mdi.AssertExpectations(t)
}

func TestTokenPoolBadName(t *testing.T) {
	em, cancel := newTestEventManager(t)
	defer cancel()
	mdi := em.database.(*databasemocks.Plugin)
	mti := &tokenmocks.Plugin{}

	pool := &fftypes.TokenPool{
		Namespace: "test-ns",
	}

	mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *fftypes.Event) bool {
		return ev.Type == fftypes.EventTypePoolRejected && ev.Reference == pool.ID && ev.Namespace == pool.Namespace
	})).Return(nil)

	info := fftypes.JSONObject{"some": "info"}
	err := em.TokenPoolCreated(mti, pool, "0x12345", info)
	assert.NoError(t, err)
	mdi.AssertExpectations(t)
}

func TestTokenPoolIDMismatch(t *testing.T) {
	em, cancel := newTestEventManager(t)
	defer cancel()
	mdi := em.database.(*databasemocks.Plugin)
	mti := &tokenmocks.Plugin{}

	pool := &fftypes.TokenPool{
		Namespace: "test-ns",
		Name:      "my-pool",
	}

	mdi.On("UpsertTokenPool", em.ctx, pool, false).Return(database.IDMismatch)
	mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *fftypes.Event) bool {
		return ev.Type == fftypes.EventTypePoolRejected && ev.Reference == pool.ID && ev.Namespace == pool.Namespace
	})).Return(nil)

	info := fftypes.JSONObject{"some": "info"}
	err := em.TokenPoolCreated(mti, pool, "0x12345", info)
	assert.NoError(t, err)
	mdi.AssertExpectations(t)
}

func TestTokenPoolUpsertFail(t *testing.T) {
	em, cancel := newTestEventManager(t)
	defer cancel()
	mdi := em.database.(*databasemocks.Plugin)
	mti := &tokenmocks.Plugin{}

	pool := &fftypes.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "test-ns",
		Name:      "my-pool",
	}

	mdi.On("UpsertTokenPool", em.ctx, pool, false).Return(fmt.Errorf("pop"))

	info := fftypes.JSONObject{"some": "info"}
	err := em.TokenPoolCreated(mti, pool, "0x12345", info)
	assert.EqualError(t, err, "pop")
}