-- Creates the schema owned by module Core / Projects & Lots.
-- Tables are added when Module 1 is implemented (see docs/decisions/0003-data-storage.md).

-- +goose Up
CREATE SCHEMA IF NOT EXISTS core;
COMMENT ON SCHEMA core IS 'Module Core / Projects & Lots';

-- +goose Down
DROP SCHEMA IF EXISTS core;
