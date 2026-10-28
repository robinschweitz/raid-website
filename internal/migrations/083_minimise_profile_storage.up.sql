-- Raw profiles have never been read by the application (SC-050).
UPDATE registration_requests SET raw_profile_json = '', level = 0, race = '', faction = '';
UPDATE characters SET external_profile_json = '';
