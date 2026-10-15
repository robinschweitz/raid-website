# One-Time Historical Import

Historical imports are deliberately offline Python operations, not website features.

Create the script for the specific source snapshot when an exceptional import is
actually required. It must:

1. Read immutable local source exports, never a live Google Sheet.
2. Require `--dry-run` before `--apply`.
3. Emit a complete machine-readable report.
4. Refuse to overwrite existing domain data.
5. Take a database backup before applying changes.

The script's exact mappings and business rules must be reviewed with the owner
for that source before it is written.
