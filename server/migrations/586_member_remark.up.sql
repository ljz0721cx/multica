-- Workspace-local display remark for a member (admin-editable nickname).
ALTER TABLE member ADD COLUMN IF NOT EXISTS remark TEXT NOT NULL DEFAULT '';
