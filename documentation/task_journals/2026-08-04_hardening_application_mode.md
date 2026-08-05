# Task Journal: Post-Merge Hardening of Application Mode

**Goal:** Perform a focused post-merge hardening and end-to-end repair of the Application Mode and Qualified Jobs feature in Career_Agent_Core.

## Next Step
- Reproduce the issues and implement the fixes.

## Plan
1. Fix Qualified-job promotion (Defect 1) by adding a job-level intent.
2. Update dashboard settings UI to use draft state and require explicit apply/confirm (Defect 2, 3).
3. Harden the settings backend API to be operationally safe (Defect 4, 5, 6).
4. Harden the Qualified Jobs APIs (Defect 7).
5. Add tests and run validations.
6. Verify defaults and wrap up.
