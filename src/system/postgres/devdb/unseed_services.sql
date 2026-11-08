-- Deletes the seed rows of seed_services.sql by id (task 015). The table stays.
DELETE FROM services WHERE id IN (1, 2, 3, 4, 5);
