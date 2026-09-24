-- 021: remove project limit — all plans get unlimited projects
UPDATE plans SET max_projects = -1;
