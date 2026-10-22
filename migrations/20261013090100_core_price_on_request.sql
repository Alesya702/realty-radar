-- "Price on request": the source publishes no price for the lot and offers it
-- on request. It is part of the versioned lot state and excludes a price (and
-- therefore a price before discount); a lot without a price and without this
-- flag simply has no stated price. No placeholder amount is ever stored.

-- +goose Up

ALTER TABLE core.lot_listing_versions
    ADD COLUMN price_on_request boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT lot_listing_versions_price_on_request_check
        CHECK (NOT price_on_request OR (price IS NULL AND price_before_discount IS NULL));

-- +goose Down

ALTER TABLE core.lot_listing_versions
    DROP CONSTRAINT lot_listing_versions_price_on_request_check,
    DROP COLUMN price_on_request;
