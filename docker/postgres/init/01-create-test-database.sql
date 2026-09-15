-- Runs once, when the postgres volume is initialized.
-- Separate database for integration tests (RR_TEST_DATABASE_URL).
CREATE DATABASE realtyradar_test OWNER realtyradar;
