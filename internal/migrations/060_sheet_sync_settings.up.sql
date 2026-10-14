CREATE TABLE sheet_sync_config (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  config_json TEXT NOT NULL,
  credentials_ciphertext TEXT NOT NULL DEFAULT ''
);
