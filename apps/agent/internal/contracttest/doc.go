// Package contracttest holds the hand-written compile/strictness checks for the generated
// vrx.v1 contract (apps/agent/gen/vrx/v1, produced by packages/proto/gen.sh). It lives outside the
// generated tree so that `pnpm gen` can wipe apps/agent/gen completely, and it is owned by the
// contract task (P03/P03b), not by the agent implementation.
//
// The checks prove the contract's central property: DesiredState is a 1:1 protobuf projection of
// the configuration document (packages/schema RootConfig), so strict protojson — unknown fields are
// errors — must accept every valid document, and explicit presence (D-039) must survive JSON.
package contracttest
