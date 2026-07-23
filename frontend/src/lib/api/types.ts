// Wire types for the Aurify API — all derived from the backend's OpenAPI spec.
//
// `schema.ts` is generated from ../backend/api/openapi.yaml via `pnpm gen:api`.
// Nothing here is hand-shaped: field shapes, required-ness, and the
// Platform/CoverStatus unions all come from the Go DTO tags (`binding:"required"`,
// `enum:"..."`). To change the contract: edit the Go DTO, run `go generate ./...`
// in the backend, then `pnpm gen:api`. See docs/adr/0008-openapi-contract.md.
import type { components } from './schema'

type Schemas = components['schemas']

export type ColorWeight = Schemas['ColorWeightResponse']
export type Playlist = Schemas['PlaylistResponse']
export type Cover = Schemas['CoverResponse']
export type GenerateCoverRequest = Schemas['GenerateCoverRequest']

/** RFC 7807 error body returned by every failing endpoint. */
export type ProblemDetails = Schemas['ProblemDetails']

// ---- authentication ----

/** The signed-in user, as returned by sign-in, refresh and GET /auth/session. */
export type Session = Schemas['SessionResponse']
export type SignUpRequest = Schemas['SignUpRequest']
export type SignInRequest = Schemas['SignInRequest']
export type ForgotPasswordRequest = Schemas['ForgotPasswordRequest']
export type ResetPasswordRequest = Schemas['ResetPasswordRequest']
export type TokenRequest = Schemas['TokenRequest']

/** Plain acknowledgement from flows that deliberately reveal nothing. */
export type MessageResponse = Schemas['MessageResponse']

/** DSP platforms — the union comes from the `enum` on the spec's platform field. */
export type Platform = GenerateCoverRequest['platform']

/** Cover lifecycle — the union comes from the `enum` on the spec's status field. */
export type CoverStatus = NonNullable<Cover['status']>
