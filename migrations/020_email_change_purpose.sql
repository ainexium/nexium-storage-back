-- 020: allow 'change_email' as a verification code purpose
ALTER TABLE verification_codes
  DROP CONSTRAINT verification_codes_purpose_check,
  ADD CONSTRAINT verification_codes_purpose_check
    CHECK (purpose IN ('verify_email', 'reset_password', 'change_email'));
