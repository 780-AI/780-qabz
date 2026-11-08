-- Seed rows of the bill table services (5 rows; task 015, as the operator gave them on 2026-11-07),
-- sorted by id. A NULL is a database NULL. ON CONFLICT (id) DO NOTHING keeps a row that already exists.
-- unseed_services.sql deletes the same ids (a unit test checks it).
INSERT INTO services ("id", "name", "type", "tag", "order", "is_active", "lang", "inquiry", "icon", "updated_at") VALUES
(1, 'برق', 'electricity', NULL, 0, true, 'fa-IR', true, NULL, '2022-02-05T10:53:35.586761+03:30'),
(2, 'گاز', 'gas', NULL, 0, true, 'fa-IR', true, NULL, '2022-02-05T10:53:35.586762+03:30'),
(3, 'تلفن همراه', 'mobile', NULL, 0, true, 'fa-IR', true, NULL, '2022-02-05T10:53:35.586762+03:30'),
(4, 'تلفن', 'tel', NULL, 0, true, 'fa-IR', true, NULL, '2022-02-05T10:53:35.586762+03:30'),
(5, 'آب', 'water', NULL, 0, true, 'fa-IR', true, NULL, '2022-02-05T10:53:35.586762+03:30')
ON CONFLICT (id) DO NOTHING;
