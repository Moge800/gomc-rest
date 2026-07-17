# CLAUDE.md

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

## 5. Record Decisions & Docs

**Keep planning and decisions in `.docs/` — update them as you go, not at the end.**

- `.docs/` is **private** (git-ignored). Never reference it from public files (README, etc.).
  Public assets (e.g. screenshots) go in a published folder like `assets/`.
- At every meaningful decision point, append to **`.docs/decisions.md`** (newest first):
  *what* was decided, *why*, and any rejected alternatives. One short entry per change is enough.
- Keep **`.docs/plan.md`** in sync when scope or approach changes.
- Record toolchain versions and update commands in **`.docs/versions.md`**; refresh after upgrades.
- When you finish a change, state what you verified (build/test/log) and note anything left unverified.

The test: someone reading `.docs/decisions.md` later can understand *why* the code looks the way it does.

## 6. Project Context

- **gomc-rest-gui** is a lightweight debugging GUI for [gomc-rest](https://github.com/Moge800/gomc-rest)
  (Mitsubishi PLC REST gateway). It is a standalone HTTP client; **never modify or embed gomc-rest**.
- Stack: **Wails v2 (Go) + React/TypeScript**. Go backend in `app.go` / `client_*.go`;
  UI in `frontend/src/` (tabs in `frontend/src/tabs/`, i18n in `frontend/src/i18n/`).
- UI is **bilingual (ja/en)** — add new strings to **both** `ja.ts` and `en.ts`.
- After changing Go bound methods, run `wails generate module`. Verify with `wails build`.
- Target is a single Windows `.exe` for closed/air-gapped networks.

## 7. Model Delegation

**Claude is the coordinator and final reviewer. Delegate suitable bounded tasks proactively without asking for permission each time.**

Use **Codex** for:
- Repository-wide investigation and implementation.
- Multi-file edits and substantial refactoring.
- Running tests, builds, and iterative fixes.
- Independent review that requires full repository context.

Use **Houtini LM with Ollama/Qwen** for:
- Bounded code generation and simple refactoring proposals.
- Unit-test drafts, boilerplate, type definitions, and documentation drafts.
- Code explanations and low-risk second-opinion reviews.
- Tasks that can be completed from explicitly provided files and instructions.

Claude must:
- Choose the appropriate delegate and provide sufficient context.
- Review and verify delegated output before applying it.
- Keep architecture, security-sensitive decisions, final edits, and final verification under Claude's control.
- Prefer Houtini for suitable local tasks and Codex when autonomous repository access, file edits, or command execution are required.
- Never send secrets or credentials to any model. Do not send private operational data to external services such as Codex.

## 8. Git Workflow

**Do not push directly to `main`** (except when the user explicitly instructs it for a
specific change — see below). Always work on a branch and merge via Pull Request.

1. Branch off `main`: `git switch -c feat/<topic>` (or `fix/<topic>`, `chore/<topic>`).
2. Commit work on that branch and push it (`git push -u origin <branch>`).
3. Open a PR: `gh pr create` (clear title + summary; reference what was verified).
4. **After the PR is merged** (by the maintainer), return to main and sync:
   `git switch main && git pull` (then delete the merged branch if desired).
- Do not force-push or rewrite history on `main`.

**Merging and releasing are the maintainer's job, not the agent's.**
- The agent prepares branches and PRs only; it must **not** merge PRs.
- The agent must **not** create releases or tags (`git tag` / `gh release`). It only
  **proposes** a version number (pre-1.0: feature → minor `0.x.0`, fix/build/docs → patch `0.x.y`).
- Direct pushes to `main` only when the user explicitly instructs it for that change.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.


---

## 9. Reliability and Context Management

**Keep tool output focused and recover from failures without restarting completed work.**

- Limit searches, logs, and file reads to the smallest useful scope.
  Prefer targeted patterns, paths, offsets, and result limits.
- Do not split normal file edits merely because a file is long.
  Use the editing method least likely to leave a partial or invalid file.
- For large investigations, use subagents or delegated tools when only
  their conclusions are needed in the main context.
- Check context usage with `/context` when a session becomes large.
  Use `/compact` to preserve relevant progress, or `/clear` when switching
  to an unrelated task.
- If a tool call or stream times out, inspect what completed, then retry
  only the failed portion with narrower input or output.
- Never repeat completed work from scratch unless its result is invalid.
- After recovering from an interrupted edit, verify the affected file
  and run the relevant build or tests.
