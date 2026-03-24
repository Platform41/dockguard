# 41 Studio — Project Lifecycle Template (v1.3)

**Status:** Draft / Active / Archived
**Repository:** [Link to Client Repo]
**Last Updated:** [Date]

------------------------------------------------------------------------

## 0. Project Metadata
*To be filled by Project Manager during intake.*

-   **Project Name:**
-   **Client:**
-   **Industry:**
-   **Plan Tier:**
-   **Project Type:** (Standard Site / SaaS / Custom App / Landing Page)
-   **Budget:**
-   **Timeline:**
-   **Repo URL:**

------------------------------------------------------------------------

# Phase 0 — Domain Dossier (Optional)
**Owner:** Insider (Domain Research Lead)
**Input:** Client Niche / Project Concept
**Output Audience:** Zero (Marketing) & One (Architecture)

## 0.1 Glossary of Power
-   **Key Terms:** (5-10 industry terms)

## 0.2 Backstage Workflow (The Invisible Work)
-   **Operational Reality:** (what happens behind the scenes)

## 0.3 Pain Matrix
-   **User Pain:**
-   **Admin Pain:**
-   **Regulatory/External Pain:**

## 0.4 Domain Dossier Summary
-   **Day in the Life:** (bulleted flow)
-   **Entity Relationship:** (who are the players?)
-   **Trap Doors:** (what usually goes wrong?)
-   **Key Metrics:** (what number matters most?)

### Phase 0 Approval
-   [ ] Dossier complete and reviewed
-   [ ] **Handover:** Brief is clear for Phase 1?

------------------------------------------------------------------------

# Phase 1 — Marketing & Strategy Brief
**Owner:** Zero (Growth Systems Architect)
**Output Audience:** One (Solution Architect) & Two (Designer)

## 1.1 Business Logic (The "Why")
-   **Primary Goal:** (e.g., "Capture leads for high-ticket consulting")
-   **Success Metric (KPI):** (e.g., "Min. 5 qualified leads/week")
-   **Current Bottleneck:**

## 1.2 Audience Profile (Malaysian Context)
-   **Target Avatar:**
-   **Buying Trigger:**
-   **Local Nuances:** (e.g., "Prefers WhatsApp over Email", "Requires BM toggle")

## 1.3 The "AI Advantage" Strategy
-   **Automated Workflows:** (e.g., "Leads auto-pushed to Google Sheets via n8n")
-   **AI Content Strategy:** (e.g., "Programmatic SEO for 50 state/area pages")

## 1.4 Content Direction
-   **Primary Hook (H1):**
-   **Objection Handling:** (What 3 fears must we address?)

### Phase 1 Approval
-   [ ] Strategy approved by Client
-   [ ] **Handover:** Brief is clear for Technical Scoping?

------------------------------------------------------------------------

# Phase 2 — Technical Scope & Architecture
**Owner:** One (Solution Architect)
**Input:** Phase 1 Brief

## 2.1 The "41 Standard" Stack
-   **Framework:** (e.g., Next.js / Astro / Rails API)
-   **Styling:** Tailwind CSS
-   **Database:** (Postgres / None / SQLite)
-   **Hosting:** (Vercel / Coolify / AWS)

## 2.2 Infrastructure & Security
-   **Containerization:** (Docker Strategy)
-   **CI/CD Pipeline:** (GitHub Actions required?)
-   **Security:** (Rate Limiting / Cloudflare Rules / PDPA Compliance)

## 2.3 Engineering Quality Gates
-   **Test Strategy:** (Unit / Integration / E2E and what must be written before implementation)
-   **Workflow Policy Reference:** (`docs/versioning-and-git-workflow.md` is the source for branch rules, release tagging, and commit discipline)
-   **Release Review Owner:** One (Solution Architect)
-   **Release Tagging Rule:** (`0.x.y` before production, `1.0.0` at first production launch, then SemVer)

## 2.4 AI Integration Architecture
-   **LLM Provider:** (OpenAI / Anthropic / Local)
-   **Token Cost Control:** (Caching strategy?)

### Phase 2 Approval
-   [ ] Feasibility Confirmed
-   [ ] Workflow policy reviewed against `docs/versioning-and-git-workflow.md`
-   [ ] **Handover:** Stack defined for Designer & Builder?

------------------------------------------------------------------------

# Phase 3 — UX & Conversion Design
**Owner:** Two (The UX Mechanic)
**Input:** Phase 1 Strategy + Phase 2 Stack

## 3.1 Conversion Logic Map
-   **Hero Goal:** (What is the ONE action here?)
-   **User Flow:** (e.g., Hero -> Trust Signals -> Problem Agitation -> WhatsApp)

## 3.2 Visual System & Tokens
-   **Typography:** (Font Family)
-   **Color Palette:** (Primary / Secondary / Accent)
-   **Radius/Spacing:** (e.g., "Rounded-lg", "Gap-4 base")

## 3.3 Wireframe Summary (Mobile First)
-   **Nav Structure:**
-   **Mobile CTA Placement:** (Sticky bottom?)

### Phase 3 Approval
-   [ ] Visuals match Marketing Goals (Phase 1)
-   [ ] Components are buildable in selected Stack (Phase 2)

------------------------------------------------------------------------

# Phase 4 — Implementation & Deployment
**Owner:** Three (Implementation Lead)
**Input:** Phase 2 Architecture + Phase 3 Design Tokens

## 4.1 Build Checklist (The "Shipper" Protocol)
-   [ ] **Scaffold:** Repo initialized with "41 Core" structure.
-   [ ] **Tests First:** Critical flows are defined in tests before or alongside implementation.
-   [ ] **Sanitization:** AI-generated code reviewed for hallucinations/security.
-   [ ] **Responsiveness:** Tested on 360px (Android) to 1920px (Desktop).
-   [ ] **Integrations:** Forms/WhatsApp/Payment Gateways connected.

## 4.2 Performance Baseline (Lighthouse)
-   **Target Score:** > 90 (Performance, SEO, Best Practices)
-   **Load Time:** < 2s on 4G

## 4.3 Deployment & Ops
-   [ ] **Environment Variables:** Set in Production (Secrets).
-   [ ] **SSL/DNS:** Propagated and Secure.
-   [ ] **Backup:** Automated backup schedule active.
-   [ ] **Workflow Compliance:** Branching, merge, and release steps follow `docs/versioning-and-git-workflow.md`.
-   [ ] **One Review:** Release candidate reviewed and approved by One before tagging.
-   [ ] **Release Tag:** Git tag created using the agreed SemVer release number.

### Phase 4 Approval
-   [ ] **STABLE:** Site is live and error-free.
-   [ ] **FAST:** Meets performance targets.

------------------------------------------------------------------------

# Phase 5 — Launch & Growth Loop
**Owner:** Zero (Marketing) + Project Manager

## 5.1 Launch Protocol
-   **Announcement:**
-   **Tracking:** GA4 / Plausible / Search Console verified.

## 5.2 30-Day Review (The Feedback Loop)
-   **Actual Traffic vs Target:**
-   **AI Automation Success Rate:** (Did the bots fail?)
-   **Next Phase Recommendations:**

------------------------------------------------------------------------
