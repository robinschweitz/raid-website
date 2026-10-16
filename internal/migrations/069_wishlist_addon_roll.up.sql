ALTER TABLE character_wishlist ADD COLUMN spec TEXT NOT NULL DEFAULT 'MAIN' CHECK (spec IN ('MAIN', 'SECOND'));
ALTER TABLE character_wishlist ADD COLUMN is_enhanced INTEGER NOT NULL DEFAULT 0 CHECK (is_enhanced IN (0, 1));
