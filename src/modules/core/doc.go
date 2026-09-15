// Package core is module 1 "Core / Projects & Lots" — the central domain model
// of the market: developers, projects (own and external in one model), buildings,
// sections, lots, data sources, lot listings and lot state history, company
// portfolio membership and directed project relations.
//
// Status: Core MVP implemented (docs/architecture/core-mvp.md). The target
// model is described in docs/architecture/core-domain-model.md; phases,
// matching candidates, completeness-based delisting and batch correction are
// not implemented yet.
//
// The root package is the module's public API for other modules: the Module
// type (use cases, including IngestObservationBatch) and type aliases of the
// contract. The implementation lives under internal/. Core depends on no other
// business module.
package core
