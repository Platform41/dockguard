# Kanban Board — [Project Name]

**Status:** Active
**Repository:** [Repo URL]
**Last Updated:** [YYYY-MM-DD]
**Board Owner:** [Persona / PM]

---

## Board Rules

- **Single source of execution:** This file is the project task board.
- **Card format:** `[Phase] Title`
- **Owner format:** `Insider / Zero / One / Two / Three / PM`
- **Priority format:** `P1 / P2 / P3`
- **WIP limit:** Max 3 cards in `Doing`.
- **Blockers:** If blocked more than 1 day, move to `Blocked` with a short reason.
- **Definition of done:** A card is done only when output is documented and handed over.
- **Test-first rule:** For implementation work, write or update tests before feature code.
- **Release rule:** Tag only after One reviews and approves the release candidate.
- **Versioning rule:** Use `0.x.y` before first production launch. Promote to `1.0.0` at first production release.

---

## Column Definitions

- **Backlog:** Valid, not yet prepared for execution.
- **This Week:** Prioritised for the current week, not yet started.
- **Doing:** Actively being worked on.
- **Review/Test:** Work complete — awaiting validation, QA, or handover.
- **Blocked:** Waiting on client, dependency, approval, or technical resolution.
- **Done:** Approved, documented, and handed over.

---

## Priority Legend

- **P1:** Revenue, launch, or production-critical
- **P2:** Important but not blocking launch
- **P3:** Nice-to-have or post-launch improvement

---

## Kanban Board

### Backlog

- [ ] `[Phase X] Task title`
  **Owner:** [Persona]
  **Priority:** [P1/P2/P3]
  **Outcome:** [Expected deliverable]
  **Source docs:** [Proposal link / Flow link / N/A]
  **Dependencies:** [None / task names]
  **Notes:** [Short note]

### This Week

- [ ] `[Phase X] Task title`
  **Owner:** [Persona]
  **Priority:** [P1/P2/P3]
  **Outcome:** [Expected deliverable]
  **Source docs:** [Proposal link / Flow link]
  **Dependencies:** [None / task names]
  **Notes:** [Short note]

### Doing

- [ ] `[Phase X] Task title`
  **Owner:** [Persona]
  **Priority:** [P1/P2/P3]
  **Started:** [YYYY-MM-DD]
  **Branch:** `feat/[branch-name]`
  **Source docs:** [Proposal link / Flow link]
  **Notes:** [Short status update]

### Review/Test

- [ ] `[Phase X] Task title`
  **Owner:** [Persona]
  **Priority:** [P1/P2/P3]
  **Review Type:** [Client / Internal / QA / Technical]
  **Branch:** `feat/[branch-name]`
  **Commit:** `[short hash]`
  **Test command:** `[exact command]`
  **Test result:** [Pass / Fail / Pending]
  **Source docs:** [Proposal link / Flow link]
  **Notes:** [Short review context]

### Blocked

- [ ] `[Phase X] Task title`
  **Owner:** [Persona]
  **Priority:** [P1/P2/P3]
  **Blocked On:** [YYYY-MM-DD]
  **Blocker:** [What is preventing progress]
  **Next Action:** [Who needs to do what]

### Done

- [x] `[Phase X] Task title`
  **Owner:** [Persona]
  **Priority:** [P1/P2/P3]
  **Completed:** [YYYY-MM-DD]
  **Branch:** `feat/[branch-name]`
  **Commit:** `[short hash]`
  **Deliverable:** [Doc / PR / Deploy / Asset]
  **Notes:** [Short completion note]

---

## Decision Log

Record any approved decisions or workflow deviations here.

| Date | Decision | Owner | Notes |
| --- | --- | --- | --- |
| [YYYY-MM-DD] | [What was decided or deviated from] | [Persona] | [e.g. direct main commit — hash abc1234] |

---

## Lifecycle Mapping

| Phase | Owner | Typical Deliverable | Target Doc |
| --- | --- | --- | --- |
| Phase 0 | Insider | Domain dossier | `docs/audience/` |
| Phase 1 | Zero | Marketing brief | `docs/audience/`, `project-lifecycle.md` |
| Phase 2 | One | Technical scope | `docs/proposals/` |
| Phase 3 | Two | UX and conversion spec | `docs/flows/` |
| Phase 4 | Two + Three | Build, QA, deployment | Code + `docs/runbooks/` |
| Phase 5 | Zero + PM | Launch review and growth loop | `project-lifecycle.md` |

---

## Starter Cards

### Backlog

- [ ] `[Phase 0] Build domain dossier`
  **Owner:** Insider
  **Priority:** P1
  **Outcome:** Industry glossary, workflow, pain matrix, trap doors
  **Source docs:** N/A
  **Dependencies:** None

- [ ] `[Phase 1] Draft marketing brief`
  **Owner:** Zero
  **Priority:** P1
  **Outcome:** KPI, buying trigger, objections, AI angle
  **Source docs:** Phase 0 output
  **Dependencies:** Phase 0 done

- [ ] `[Phase 2] Write architecture proposal`
  **Owner:** One
  **Priority:** P1
  **Outcome:** Stack, infra, security, performance targets
  **Source docs:** `docs/proposals/one-<project>-proposal.md`
  **Dependencies:** Phase 1 done

- [ ] `[Phase 3] Write UX flow spec`
  **Owner:** Two
  **Priority:** P1
  **Outcome:** Conversion map, design tokens, mobile wireframe
  **Source docs:** `docs/flows/two-<project>-flow.md`
  **Dependencies:** Phase 1, Phase 2 done

- [ ] `[Phase 4] Write tests for implementation`
  **Owner:** Three
  **Priority:** P1
  **Outcome:** Failing tests for critical flows before feature code
  **Source docs:** Proposal + Flow links
  **Dependencies:** Phase 2, Phase 3 done

- [ ] `[Phase 4] Implement frontend`
  **Owner:** Two
  **Priority:** P1
  **Outcome:** Tailwind components, responsive layout, CTAs
  **Source docs:** `docs/flows/two-<project>-flow.md`
  **Dependencies:** Tests written

- [ ] `[Phase 4] Implement backend and integrations`
  **Owner:** Three
  **Priority:** P1
  **Outcome:** API, database, auth, third-party integrations
  **Source docs:** `docs/proposals/one-<project>-proposal.md`
  **Dependencies:** Tests written

- [ ] `[Phase 4] Review release candidate`
  **Owner:** One
  **Priority:** P1
  **Outcome:** Architecture and release risk reviewed before tagging
  **Source docs:** Phase 4 checklist in `project-lifecycle.md`
  **Dependencies:** Implementation done

- [ ] `[Phase 4] Tag and deploy`
  **Owner:** One
  **Priority:** P1
  **Outcome:** Git tag, deployment verified, health check passed
  **Source docs:** `docs/runbooks/deploy.md`
  **Dependencies:** One review approved

- [ ] `[Phase 5] Run launch review`
  **Owner:** Zero / PM
  **Priority:** P2
  **Outcome:** Analytics verified, 30-day feedback loop started
  **Source docs:** `project-lifecycle.md` Phase 5
  **Dependencies:** Phase 4 done

---

## Weekly Cadence

- **Monday:** Move ready work into `This Week`, confirm owners, clear blockers.
- **Midweek:** Update card notes with concrete progress — not vague status.
- **Friday:** Close completed cards, move review outcomes, prepare next week.

---

## Change Log

| Date | Change |
| --- | --- |
| [YYYY-MM-DD] | Board initialized |
