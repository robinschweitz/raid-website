-- Every raid series uses the realm's server time (ADR 114).
UPDATE raid_series SET timezone = 'Europe/Berlin' WHERE timezone <> 'Europe/Berlin';
