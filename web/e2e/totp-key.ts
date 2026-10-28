// Public test-only key shared by the e2e server and its database fixture.
export const e2eTOTPKey = process.env.TOTP_ENCRYPTION_KEY || Buffer.alloc(32, 0x65).toString('base64')
