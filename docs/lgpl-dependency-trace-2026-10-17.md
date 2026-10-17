# LGPL dependency trace: n8n and personal portal

Date: 2026-10-17
Scope: read-only inspection of `/opt/n8n-test` dependency metadata and code, the n8n workflow database, and `/opt/personal-portal/frontend` source/build output. No packages were removed or changed.

## Executive result

The four packages marked `LGPL-3.0-or-later` are **not part of the personal portal frontend dependency tree or its built static assets**. They are installed transitively under the n8n installation through the Milvus integration dependency chain. Static inspection and an isolated Node.js module-load test indicate that these four packages are not loaded when the n8n Milvus node module and its Node.js SDK entry path are imported. The separate browser bundles inside the `@shanghaikid/parquetjs` package contain references to these packages, but no corresponding references were found in the n8n editor's built static assets or the personal portal build.

The n8n database contained 14 workflows (1 active) at the time of inspection; none had a node whose type matched `milvus`. This supports the conclusion that the Milvus integration is not currently used by stored workflows, but it does not prove that no future workflow, manual code path, or external package consumer could load it.

No package was deleted. The evidence does not establish an LGPL violation.

## Dependency chain

```text
n8n@2.42.6
└─ @n8n/n8n-nodes-langchain@2.42.4
   └─ @zilliz/milvus2-sdk-node@2.6.17
      └─ @shanghaikid/parquetjs@1.8.8 (MIT)
         └─ @zenfs/core@2.7.7 (LGPL-3.0-or-later)
            ├─ @zenfs/streams@1.0.1 (LGPL-3.0-or-later)
            ├─ memium@1.0.4 (LGPL-3.0-or-later)
            │  └─ kerium@1.5.0
            │     └─ utilium@3.9.0 (LGPL-3.0-or-later)
            └─ utilium@3.9.0 (LGPL-3.0-or-later)
```

The installed package metadata confirms these versions and SPDX-like license labels. Each package directory contains a `LICENSE.md` file. `npm explain` also shows optional peer paths to the Milvus SDK from LangChain packages; the direct chain above is the concrete installed path found through n8n's LangChain node package.

## Actual-use checks

- Personal portal: searched `frontend/package.json`, `package-lock.json`, `src`, `public`, and `dist`; none references the four packages, `@shanghaikid/parquetjs`, or `@zilliz/milvus2-sdk-node`.
- Personal portal build: 44 files in `frontend/dist` at the time of inspection; no built JS/CSS/HTML file matched these package names.
- n8n source: the Milvus node and its list-search helper import `@zilliz/milvus2-sdk-node`, so the optional integration is installed and implemented in the server-side node package.
- Isolated module-load test: importing `VectorStoreMilvus.node.js` loaded `@zilliz/milvus2-sdk-node` and `@shanghaikid/parquetjs` into that test process's `require.cache`, but did **not** load `@zenfs/core`, `@zenfs/streams`, `memium`, or `utilium`.
- The Node entrypoint `@shanghaikid/parquetjs/dist/parquet.js` has no imports of those LGPL packages. The package's separate `dist/browser/parquet.js`, `parquet.esm.js`, and `parquet.cjs.js` bundles do contain references, but they were not found in the n8n editor static distribution or personal portal build.
- Workflow database: 14 workflows total, 1 active, 0 nodes with a type containing `milvus`.
- The live n8n process was not instrumented or restarted for this audit. The isolated module-load test and stored workflow inspection are evidence, not a complete runtime trace of every possible dynamic path.

## Distribution assessment

- The personal portal serves its own frontend build; these packages are not in that build. No LGPL package code was found in the portal's public/static output.
- The four packages reside in n8n's server-side `node_modules`. A package's presence on a server does not by itself mean its source or binary is being distributed to website visitors.
- No evidence was found that these packages are delivered as downloadable software or embedded in a customer-facing portal artifact. The n8n web UI itself is served as a service; whether n8n is offered to external customers and the separate n8n Sustainable Use License issue remain distinct questions.
- On current evidence, there is no identified need to add these four packages to the personal portal's `THIRD_PARTY_NOTICES.txt` or publish their source code as part of the portal. Reassess if a future build embeds these packages, or if software containing them is distributed to customers.

## Recommended disposition

1. Keep the packages in place; do not remove transitive dependencies by hand.
2. Do not treat the four package labels alone as evidence of infringement.
3. If Milvus support is not required, assess removal only through a supported n8n package/deployment change, followed by regression tests—not by deleting files from `node_modules`.
4. If the product later redistributes n8n or any artifact containing these packages, perform a distribution-specific license review and include the required license/source notices for the exact shipped components.

This is a technical dependency trace, not a legal opinion. The separate n8n Sustainable Use License and enterprise-license review remains open.
