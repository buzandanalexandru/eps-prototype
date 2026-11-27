-- Better Auth tables (U1), generated with Better Auth's own migration planner
-- (scripts/auth-migration-sql.mjs) for our plugin set: magic link, JWT (jwks), database rate limit.
-- All are platform tables (no organiser_id); they are on the allowlist in
-- services/internal/tenancy/allowlist.go.

-- +goose Up
create table control."user" ("id" text not null primary key, "name" text not null, "email" text not null unique, "emailVerified" boolean not null, "image" text, "createdAt" timestamptz default CURRENT_TIMESTAMP not null, "updatedAt" timestamptz default CURRENT_TIMESTAMP not null);

create table control."session" ("id" text not null primary key, "expiresAt" timestamptz not null, "token" text not null unique, "createdAt" timestamptz default CURRENT_TIMESTAMP not null, "updatedAt" timestamptz not null, "ipAddress" text, "userAgent" text, "userId" text not null references control."user" ("id") on delete cascade);

create table control."account" ("id" text not null primary key, "accountId" text not null, "providerId" text not null, "userId" text not null references control."user" ("id") on delete cascade, "accessToken" text, "refreshToken" text, "idToken" text, "accessTokenExpiresAt" timestamptz, "refreshTokenExpiresAt" timestamptz, "scope" text, "password" text, "createdAt" timestamptz default CURRENT_TIMESTAMP not null, "updatedAt" timestamptz not null);

create table control."verification" ("id" text not null primary key, "identifier" text not null, "value" text not null, "expiresAt" timestamptz not null, "createdAt" timestamptz default CURRENT_TIMESTAMP not null, "updatedAt" timestamptz default CURRENT_TIMESTAMP not null);

create table control."jwks" ("id" text not null primary key, "publicKey" text not null, "privateKey" text not null, "createdAt" timestamptz not null, "expiresAt" timestamptz, "alg" text, "crv" text);

create table control."rateLimit" ("id" text not null primary key, "key" text not null unique, "count" integer not null, "lastRequest" bigint not null);

create index "session_userId_idx" on control."session" ("userId");

create index "account_userId_idx" on control."account" ("userId");

create index "verification_identifier_idx" on control."verification" ("identifier");

-- The Go core verifies core tokens with the public keys only (design D0.4): a column grant, so the
-- core role can never read the (encrypted) private keys.
GRANT SELECT ("id", "publicKey", "createdAt", "expiresAt", "alg", "crv") ON control."jwks" TO eps_core;
