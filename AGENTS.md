# AGENTS.md

These rules apply to all agent-made changes in this repository.

## PR Gate

- Before opening or updating a PR, run the same local gates as `.github/workflows/quality-gates.yml`.
- Required commands:
  - `./scripts/lint.sh`
  - `./tests/scripts/check-refactor-line-gate.sh`
  - `./tests/scripts/run-unit-all.sh`
  - `npm run build --prefix webui`
- The CI WebUI build runs `npm ci --prefix webui --prefer-offline --no-audit` before the build; run that install step locally when dependencies are missing, stale, or package-lock changed.
- For high-risk protocol, streaming, release, or real-account path changes, also run `./tests/scripts/run-live.sh` when credentials/config are available. Do not share unredacted artifacts from `artifacts/testsuite/`.

## Go Lint Rules

- Run `gofmt -w` on every changed Go file before commit or push.
- Do not ignore error returns from I/O-style cleanup calls such as `Close`, `Flush`, `Sync`, or similar methods.
- If a cleanup error cannot be returned, log it explicitly.

## Change Scope

- Keep changes additive and tightly scoped to the requested feature or bugfix.
- Do not mix unrelated refactors into feature PRs unless they are required to make the change pass gates.

## Protocol Adapter Boundary

- Do not let OpenAI Chat, OpenAI Responses, Claude, Gemini, or other interface protocol formatting own shared business behavior.
- Normalize protocol-specific request shapes into the project standard request/turn model first, run shared business logic in one place, then render back to the target protocol at the boundary.
- Business logic that must stay globally consistent includes empty-output retry, thinking/reasoning handling, tool-call detection and policy, usage accounting, current-input-file injection, history persistence, file/reference handling, and completion payload assembly.
- If a behavior must differ by protocol, keep the difference as an explicit adapter/rendering concern and document why it cannot live in the shared normalized path.

## Documentation Sync

- When business logic or user-visible behavior changes, update the corresponding documentation in the same change.
- `docs/prompt-compatibility.md` is the source-of-truth document for the “API -> pure-text web-chat context” compatibility flow.
- If a change affects message normalization, tool prompt injection, prompt-visible tool history, file/reference handling, history split, or completion payload assembly, update `docs/prompt-compatibility.md` in the same change.
- If a change affects tool-call parsing, streaming/sieve behavior, prompt-visible tool semantics, or tool-call policy, update `docs/toolcall-semantics.md` in the same change.
- If a change adds, removes, moves, or materially changes modules, routes, or runtime responsibilities, update `docs/ARCHITECTURE.md` and `docs/ARCHITECTURE.en.md` as appropriate.
- If a change affects public API behavior, update `API.md` and `API.en.md` as appropriate.

## Branch Discipline

- Never commit or push directly to `main`. `main` is integration-only and accepts changes via PR.
- If the working tree is on `main`, the agent must first create and switch to a topic branch before making any edits or running write actions.
- Always create a topic branch from the latest `main`. Use the naming convention defined in `docs/dev-roadmap.md`:
  - `feat/<milestone>-<theme>-<short-slug>` (e.g. `feat/m1-toolparser-confidence-rework`)
  - `fix/<theme>-<short-slug>`
  - `docs/<theme>-<short-slug>`
  - `refactor/<theme>-<short-slug>`
  - `chore/<short-slug>`
  - Theme enum: `toolparser`, `context`, `governance`, `webui`, `infra`, `docs`. Milestone enum: `m0`–`m4`.
- One milestone or one logical change per branch. Do not accumulate unrelated commits.
- Before opening a PR, rebase on `main` and run the PR Gate commands listed above.
- Squash-merge with a semantic-prefix commit message (`feat:`, `fix:`, `docs:`, `refactor:`, `chore:`, `perf:`, `style:`).
- Wait for the previous milestone branch to be merged before opening the next milestone branch.
