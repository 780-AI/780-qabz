-- Seed rows of the bill table fees (1 row; task 039): the default fee of bill (9000, fixed) for the
-- order provider of the Irancell mobile bill (providers row 26, irancell / order), so that an order in
-- development takes its fee from a fees row (orders.fee_id is set). The id is high so that it does not
-- collide with a row added by hand. ON CONFLICT (id) DO NOTHING keeps a row that already exists.
-- created_at and updated_at get the column defaults. unseed_fees.sql deletes the same ids (a unit test checks it).
INSERT INTO fees ("id", "title", "provider_id", "type", "config", "user_id", "deleted_at") VALUES
(900001, 'irancell order (dev seed)', 26, 'fixed', '{"amount": 9000}', NULL, NULL)
ON CONFLICT (id) DO NOTHING;
