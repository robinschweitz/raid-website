function base64URLToBuffer(value: string): ArrayBuffer {
  const base64 = value.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - value.length % 4) % 4)
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i)
  return bytes.buffer
}

function bufferToBase64URL(value: ArrayBuffer | null): string {
  if (!value) return ''
  const bytes = new Uint8Array(value)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
}

interface CredentialDescriptorJSON {
  id: string
  type: PublicKeyCredentialType
  transports?: AuthenticatorTransport[]
}

interface CreationOptionsJSON {
  publicKey: Omit<PublicKeyCredentialCreationOptions, 'challenge' | 'user' | 'excludeCredentials'> & {
    challenge: string
    user: Omit<PublicKeyCredentialUserEntity, 'id'> & { id: string }
    excludeCredentials?: CredentialDescriptorJSON[]
  }
}

interface RequestOptionsJSON {
  publicKey: Omit<PublicKeyCredentialRequestOptions, 'challenge' | 'allowCredentials'> & {
    challenge: string
    allowCredentials?: CredentialDescriptorJSON[]
  }
}

export interface WebAuthnBeginResponse<TOptions> {
  session: string
  options: TOptions
}

export interface SerializedWebAuthnCredential {
  id: string
  rawId: string
  type: string
  authenticatorAttachment: AuthenticatorAttachment | null
  clientExtensionResults: AuthenticationExtensionsClientOutputs
  response: Record<string, string | string[]>
}

export function browserSupportsWebAuthn(): boolean {
  return typeof window !== 'undefined' && 'PublicKeyCredential' in window && !!navigator.credentials
}

export async function createWebAuthnCredential(options: CreationOptionsJSON): Promise<SerializedWebAuthnCredential> {
  const publicKey: PublicKeyCredentialCreationOptions = {
    ...options.publicKey,
    challenge: base64URLToBuffer(options.publicKey.challenge),
    user: { ...options.publicKey.user, id: base64URLToBuffer(options.publicKey.user.id) },
    excludeCredentials: options.publicKey.excludeCredentials?.map((credential) => ({ ...credential, id: base64URLToBuffer(credential.id) })),
  }
  const credential = await navigator.credentials.create({ publicKey }) as PublicKeyCredential | null
  if (!credential) throw new Error('security key registration cancelled')
  return serializeCredential(credential)
}

export async function getWebAuthnCredential(options: RequestOptionsJSON): Promise<SerializedWebAuthnCredential> {
  const publicKey: PublicKeyCredentialRequestOptions = {
    ...options.publicKey,
    challenge: base64URLToBuffer(options.publicKey.challenge),
    allowCredentials: options.publicKey.allowCredentials?.map((credential) => ({ ...credential, id: base64URLToBuffer(credential.id) })),
  }
  const credential = await navigator.credentials.get({ publicKey }) as PublicKeyCredential | null
  if (!credential) throw new Error('security key login cancelled')
  return serializeCredential(credential)
}

function serializeCredential(credential: PublicKeyCredential): SerializedWebAuthnCredential {
  const response = credential.response as AuthenticatorAttestationResponse | AuthenticatorAssertionResponse
  const result: SerializedWebAuthnCredential = {
    id: credential.id,
    rawId: bufferToBase64URL(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment as AuthenticatorAttachment | null,
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      clientDataJSON: bufferToBase64URL(response.clientDataJSON),
    },
  }
  if ('attestationObject' in response) {
    result.response.attestationObject = bufferToBase64URL(response.attestationObject)
    result.response.transports = response.getTransports?.() ?? []
  }
  if ('authenticatorData' in response) {
    result.response.authenticatorData = bufferToBase64URL(response.authenticatorData)
    result.response.signature = bufferToBase64URL(response.signature)
    result.response.userHandle = bufferToBase64URL(response.userHandle)
  }
  return result
}
