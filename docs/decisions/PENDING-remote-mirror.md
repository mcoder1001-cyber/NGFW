# PENDING: remote-mirror

- raised: 2026-09-24 by the manager (ngfw-46), from the REVIEW-2026-09-24 report items 5.1a and 5.3 (verified, D-125)
- decision: **<empty until the product owner fills it>**
- parked tasks: none as whole rows. Two things wait: the 5.3 part of TD-18 (a `.gitlab-ci.yml` wrapper, only if the forge is GitLab) and the `docs/contributing.md` fix.

## Context
- The repo's `origin` is `https://git.amnafzar.ir/eshaghi/ngfw.git`. It holds `origin/main` at 3364aca (2026-09-23 11:40), about 760 commits behind main: 758 when the review ran, 764 at 35f3a96. `packages/` and everything after the docs import are missing there.
- Main itself is complete. Only the mirror is behind.
- `.github/workflows/ci.yml` exists on main (c09583c) but was never pushed. By design, CI is local: `tools/ci.sh` plus the `pre-merge-commit` hook.
- `docs/contributing.md:3` and `:192-195` say "no remote", which no longer matches the repo.
- Pushing product code to an outside host is a choice about where the code lives, so it needs you.

## Questions for the product owner
- (a) Should `main` and the `contracts-v1` tag be mirrored to git.amnafzar.ir after every merge?
- (b) Which forge is it: Gitea/Forgejo or GitLab?
- (c) Is product code allowed on git.amnafzar.ir?

## Options
| # | Option | Cost now (agent-h) | Reversal cost (agent-h) | Risk |
|---|---|---|---|---|
| 1 | Mirror `main` and the tags after every merge. First run gitleaks over the full history (today `tools/ci.sh` scans only branch commits). Add a push step to MANAGER-PROMPT §2 and fix contributing.md. CI on the forge: a Gitea/Forgejo runner with an `ubuntu-latest` label (no code), or TD-18's thin `.gitlab-ci.yml` wrapper (~1 h) | ~1 + ~1 min per merge | pushed history cannot be taken back | low once the history scan is clean |
| 2 | Local only: remove or rename the `origin` remote and make contributing.md say so | 0.2 | 0.2 | no off-host copy of the code |
| 3 | The product owner pushes by hand when wanted; contributing.md describes the mirror as a snapshot | 0.2 | 0.2 | the mirror stays stale between pushes |

## Recommendation
Option 1, after a clean gitleaks scan of the full history. It gives an off-host copy and makes the existing workflow file useful at no ongoing cost.

## What continues meanwhile
Everything continues. The local gate (`tools/ci.sh` plus the hook) stays the merge gate either way.

## خلاصهٔ فارسی
- **مسئله:** مخزن راه دور (git.amnafzar.ir) حدود ۷۶۰ commit از `main` عقب است. خود مخزن اصلی کامل است و فقط آینه عقب مانده. متن contributing.md هم می‌گوید «remote نداریم».
- **سه پرسش از شما:** (الف) آیا بعد از هر merge، شاخهٔ `main` و تگ `contracts-v1` به آن‌جا فرستاده شود؟ (ب) آن سرور Gitea/Forgejo است یا GitLab؟ (ج) آیا گذاشتن کد محصول روی آن سرور مجاز است؟
- **پیشنهاد:** بله. پیش از اولین ارسال، کل تاریخچه با gitleaks بررسی شود.
- **تا تصمیم شما:** هیچ کاری متوقف نمی‌شود. فقط فایل CI مخصوص GitLab و اصلاح contributing.md منتظر می‌مانند.
- **تصمیم لازم است:** لطفاً در خط `decision:` بنویسید یا در چت بگویید.
