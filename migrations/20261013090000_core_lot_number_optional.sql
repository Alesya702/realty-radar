-- The lot number is optional (NULL when the source states none). A missing
-- number is not an identity: lots without a number are not matched by
-- placement (key K2) and are not merged with each other; a UI text such as
-- «Не указан» is never stored as a number.

-- +goose Up

ALTER TABLE core.lots ALTER COLUMN number DROP NOT NULL;

-- +goose Down

-- The previous schema requires a number. Lots without one get their own id,
-- which is unique, so rolling back never merges them under a shared
-- placeholder such as «Не указан».
UPDATE core.lots SET number = id::text WHERE number IS NULL;
ALTER TABLE core.lots ALTER COLUMN number SET NOT NULL;
