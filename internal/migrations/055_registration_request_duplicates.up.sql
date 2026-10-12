CREATE TRIGGER registration_requests_block_duplicate_active
BEFORE INSERT ON registration_requests
WHEN EXISTS (
  SELECT 1
  FROM registration_requests
  WHERE external_source = NEW.external_source
    AND external_profile_id = NEW.external_profile_id
    AND status IN ('PENDING', 'APPROVED', 'CONSUMED')
)
BEGIN
  SELECT RAISE(ABORT, 'active_registration_request_exists');
END;
