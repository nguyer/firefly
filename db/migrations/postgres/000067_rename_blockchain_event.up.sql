BEGIN;
UPDATE events SET etype='bockchain_event_received' WHERE etype='bockchain_event';
COMMIT;