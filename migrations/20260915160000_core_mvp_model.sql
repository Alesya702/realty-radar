-- Core MVP model (docs/architecture/core-mvp.md): developers, projects,
-- buildings, sections, data sources, lots, lot listings, ingestion batches,
-- lot listing versions (history), portfolio memberships, project relations.
-- Business dictionaries without approved values are not created here (A-19..A-21).

-- +goose Up

CREATE TABLE core.developers (
    id         uuid PRIMARY KEY,
    name       text NOT NULL CHECK (btrim(name) <> ''),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE core.projects (
    id           uuid PRIMARY KEY,
    developer_id uuid REFERENCES core.developers (id),
    name         text NOT NULL CHECK (btrim(name) <> ''),
    city         text,
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL
);
CREATE INDEX projects_developer_id_idx ON core.projects (developer_id);

CREATE TABLE core.buildings (
    id         uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES core.projects (id),
    name       text NOT NULL CHECK (btrim(name) <> ''),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (project_id, name),
    -- target of the composite FK that keeps lot.building inside lot.project
    UNIQUE (id, project_id)
);

CREATE TABLE core.sections (
    id          uuid PRIMARY KEY,
    building_id uuid NOT NULL REFERENCES core.buildings (id),
    name        text NOT NULL CHECK (btrim(name) <> ''),
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    UNIQUE (building_id, name),
    UNIQUE (id, building_id)
);

CREATE TABLE core.data_sources (
    id         uuid PRIMARY KEY,
    code       text NOT NULL UNIQUE CHECK (code ~ '^[a-z0-9][a-z0-9_-]{1,62}$'),
    name       text NOT NULL CHECK (btrim(name) <> ''),
    kind       text NOT NULL CHECK (kind IN ('manual', 'document_channel')),
    is_active  boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

-- Reference row: the data source of manual entry (domain.ManualDataSourceCode).
INSERT INTO core.data_sources (id, code, name, kind, is_active, created_at, updated_at)
VALUES ('01995a6e-0000-7000-8000-000000000001', 'manual', 'Ручной ввод', 'manual', true, now(), now());

CREATE TABLE core.lots (
    id                 uuid PRIMARY KEY,
    project_id         uuid NOT NULL REFERENCES core.projects (id),
    building_id        uuid,
    section_id         uuid,
    lot_type           text NOT NULL CHECK (lot_type IN ('apartment', 'commercial', 'storage', 'parking')),
    number             text NOT NULL CHECK (btrim(number) <> ''),
    floor              smallint,
    primary_listing_id uuid,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL,
    -- building (if any) belongs to the lot's project; section (if any) to the lot's building
    FOREIGN KEY (building_id, project_id) REFERENCES core.buildings (id, project_id),
    FOREIGN KEY (section_id, building_id) REFERENCES core.sections (id, building_id),
    CHECK (section_id IS NULL OR building_id IS NOT NULL)
);
CREATE INDEX lots_project_id_idx ON core.lots (project_id);
CREATE INDEX lots_building_id_idx ON core.lots (building_id);

CREATE TABLE core.lot_listings (
    id             uuid PRIMARY KEY,
    lot_id         uuid NOT NULL REFERENCES core.lots (id),
    data_source_id uuid NOT NULL REFERENCES core.data_sources (id),
    source_key     text,
    source_url     text,
    published_at   date,
    first_seen_at  timestamptz NOT NULL,
    last_seen_at   timestamptz NOT NULL,
    created_at     timestamptz NOT NULL,
    updated_at     timestamptz NOT NULL,
    UNIQUE (lot_id, data_source_id),
    UNIQUE (data_source_id, source_key),
    UNIQUE (id, lot_id),
    CHECK (last_seen_at >= first_seen_at)
);

-- The primary listing of a lot must be one of its own listings.
ALTER TABLE core.lots
    ADD CONSTRAINT lots_primary_listing_fk
    FOREIGN KEY (primary_listing_id, id) REFERENCES core.lot_listings (id, lot_id);

CREATE TABLE core.ingestion_batches (
    id             uuid PRIMARY KEY,
    data_source_id uuid NOT NULL REFERENCES core.data_sources (id),
    origin_kind    text NOT NULL CHECK (origin_kind IN ('manual_entry', 'document_import')),
    -- id of the confirmed import/entry in the sending module; no FK across modules
    origin_ref     uuid,
    observed_at    timestamptz NOT NULL,
    received_at    timestamptz NOT NULL,
    UNIQUE (origin_kind, origin_ref)
);

CREATE TABLE core.lot_listing_versions (
    id                    uuid PRIMARY KEY,
    listing_id            uuid NOT NULL REFERENCES core.lot_listings (id),
    valid_from            timestamptz NOT NULL,
    valid_to              timestamptz,
    last_observed_at      timestamptz NOT NULL,
    presence              text NOT NULL CHECK (presence IN ('listed', 'delisted')),
    status                text NOT NULL CHECK (status IN ('available', 'reserved', 'sold', 'unknown')),
    rooms                 smallint CHECK (rooms >= 0),
    area_total            numeric(10, 2) CHECK (area_total > 0),
    price                 numeric(15, 2) CHECK (price > 0),
    price_before_discount numeric(15, 2),
    finish                text CHECK (finish IS NULL OR btrim(finish) <> ''),
    batch_id              uuid NOT NULL REFERENCES core.ingestion_batches (id),
    created_at            timestamptz NOT NULL,
    CHECK (valid_to IS NULL OR valid_to > valid_from),
    CHECK (last_observed_at >= valid_from AND (valid_to IS NULL OR last_observed_at < valid_to)),
    -- price_before_discount only with an explicit discount (Q-CORE-15)
    CHECK (price_before_discount IS NULL OR (price IS NOT NULL AND price_before_discount > price)),
    UNIQUE (listing_id, valid_from)
);
-- exactly one current version per listing
CREATE UNIQUE INDEX lot_listing_versions_current_idx ON core.lot_listing_versions (listing_id) WHERE valid_to IS NULL;

CREATE TABLE core.portfolio_memberships (
    id         uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES core.projects (id),
    valid_from date,
    valid_to   date,
    note       text,
    created_at timestamptz NOT NULL,
    CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from)
);
CREATE INDEX portfolio_memberships_project_id_idx ON core.portfolio_memberships (project_id);

CREATE TABLE core.project_relations (
    id                 uuid PRIMARY KEY,
    subject_project_id uuid NOT NULL REFERENCES core.projects (id),
    related_project_id uuid NOT NULL REFERENCES core.projects (id),
    relation_type      text NOT NULL CHECK (relation_type IN ('direct_competitor', 'indirect_competitor', 'reference')),
    rationale          text,
    valid_from         date NOT NULL,
    valid_to           date,
    created_at         timestamptz NOT NULL,
    CHECK (subject_project_id <> related_project_id),
    CHECK (valid_to IS NULL OR valid_to >= valid_from)
);
CREATE INDEX project_relations_subject_idx ON core.project_relations (subject_project_id);
CREATE INDEX project_relations_related_idx ON core.project_relations (related_project_id);

-- +goose Down

DROP TABLE core.project_relations;
DROP TABLE core.portfolio_memberships;
DROP TABLE core.lot_listing_versions;
DROP TABLE core.ingestion_batches;
ALTER TABLE core.lots DROP CONSTRAINT lots_primary_listing_fk;
DROP TABLE core.lot_listings;
DROP TABLE core.lots;
DROP TABLE core.data_sources;
DROP TABLE core.sections;
DROP TABLE core.buildings;
DROP TABLE core.projects;
DROP TABLE core.developers;
