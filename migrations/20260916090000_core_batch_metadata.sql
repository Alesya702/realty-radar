-- Provenance of an ingestion batch (ADR-0007 п. 2, core-domain-model.md 8.2-8.3):
-- the basis of the observation moment, the scope and completeness of the batch,
-- the moment the user confirmed it, and a hash of the batch content.
-- The hash makes the idempotency key origin_ref safe: the same reference with
-- different content is a conflict, not a repeat.
-- Only partial batches are accepted for now: complete implies delisting by
-- absence, which is not implemented (Q-CORE-17).

-- +goose Up

ALTER TABLE core.ingestion_batches
    ADD COLUMN observed_at_basis text NOT NULL DEFAULT 'entry_time'
        CHECK (observed_at_basis IN ('document_date', 'received_date', 'upload_time', 'entry_time')),
    ADD COLUMN scope_building_id uuid REFERENCES core.buildings (id),
    ADD COLUMN scope_lot_types text[]
        CHECK (scope_lot_types IS NULL
               OR (cardinality(scope_lot_types) > 0
                   AND scope_lot_types <@ ARRAY['apartment', 'commercial', 'storage', 'parking'])),
    ADD COLUMN completeness text NOT NULL DEFAULT 'partial' CHECK (completeness IN ('partial')),
    ADD COLUMN confirmed_at timestamptz,
    -- nullable: batches accepted before this migration have no known hash
    ADD COLUMN content_hash bytea CHECK (content_hash IS NULL OR octet_length(content_hash) = 32);

-- The defaults only fill the rows that already exist; the application always
-- states the basis and the completeness.
ALTER TABLE core.ingestion_batches
    ALTER COLUMN observed_at_basis DROP DEFAULT,
    ALTER COLUMN completeness DROP DEFAULT;

CREATE INDEX ingestion_batches_scope_building_idx ON core.ingestion_batches (scope_building_id);

-- +goose Down

DROP INDEX core.ingestion_batches_scope_building_idx;

ALTER TABLE core.ingestion_batches
    DROP COLUMN content_hash,
    DROP COLUMN confirmed_at,
    DROP COLUMN completeness,
    DROP COLUMN scope_lot_types,
    DROP COLUMN scope_building_id,
    DROP COLUMN observed_at_basis;
