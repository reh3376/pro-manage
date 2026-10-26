# USTS specs

Security behaviour contracts: every protected endpoint rejects unauthenticated and wrong-role callers, contributor scoping within a project, share-token exposure, injection, rate limiting. Empty so far. The auth boundary now exists (SSO sessions and the dev header since slice 4b, the section-level ACL since slice 5, the provider webhook since slice 6), so the first USTS specs are the next test-governance item (backlog B-14); merge-blocking before the pilot (FSD §12, framework matrix).
