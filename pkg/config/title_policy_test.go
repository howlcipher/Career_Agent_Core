package config

import "testing"

// targetTrackRoles mirrors the shape of profile.yaml's configured roles for
// the primary/adjacent tracks this policy exists to protect: DevOps,
// Platform, SRE, Infrastructure, Cloud, and AI infrastructure. It is
// intentionally the same small set TitleEligible's distinctive-word/phrase
// matching already recognizes, so these tests exercise the policy layer
// (management exclusion, seniority stretch) independently of the exact
// wording an operator's real role list happens to use.
var targetTrackRoles = []string{
	"DevOps Engineer",
	"Senior DevOps Engineer",
	"Azure DevOps Engineer",
	"Platform Engineer",
	"Senior Platform Engineer",
	"Cloud Platform Engineer",
	"Site Reliability Engineer",
	"Senior Site Reliability Engineer",
	"SRE",
	"Senior SRE",
	"SRE Engineer",
	"Infrastructure Engineer",
	"Infrastructure Automation Engineer",
	"Cloud Infrastructure Engineer",
	"Cloud Engineer",
	"Production Engineer",
	"Production Support Engineer",
	"AI Infrastructure Engineer",
}

func TestClassifyTitle_PrimaryAccepted(t *testing.T) {
	titles := []string{
		"Senior DevOps Engineer",
		"DevOps Engineer",
		"Azure DevOps Engineer",
		"Senior Platform Engineer",
		"Platform Engineer",
		"Senior Site Reliability Engineer",
		"Cloud Platform Engineer",
		// Bare-acronym SRE titles (independent review Reviewer B, task
		// instructions #556): "sre" is a distinctive word but no phrase in
		// targetTrackRoles spells out the acronym on its own, only "Site
		// Reliability Engineer" (sharing "reliability"), so these need
		// their own explicit role phrase to match at all.
		"Senior SRE",
		"SRE Engineer",
	}
	for _, title := range titles {
		cls := ClassifyTitle(title, targetTrackRoles, false, true)
		if cls.Fit == FitReject {
			t.Errorf("ClassifyTitle(%q) = reject (%s), want primary/adjacent acceptance", title, cls.Reason)
		}
		if !TitleEligibleForRoles(title, targetTrackRoles, false, true) {
			t.Errorf("TitleEligibleForRoles(%q) = false, want true", title)
		}
	}
}

func TestClassifyTitle_AdjacentAccepted(t *testing.T) {
	titles := []string{
		"Infrastructure Automation Engineer",
		"Production Engineer",
		"Cloud Infrastructure Engineer",
		"AI Infrastructure Engineer",
	}
	for _, title := range titles {
		cls := ClassifyTitle(title, targetTrackRoles, false, true)
		if cls.Fit == FitReject {
			t.Errorf("ClassifyTitle(%q) = reject (%s), want adjacent acceptance", title, cls.Reason)
		}
	}
}

func TestClassifyTitle_StretchAccepted(t *testing.T) {
	titles := []string{
		"Staff DevOps Engineer",
		"Principal DevOps Engineer",
		"Principal Platform Engineer",
		"Principal Site Reliability Engineer",
	}
	for _, title := range titles {
		cls := ClassifyTitle(title, targetTrackRoles, false, true)
		if cls.Fit != FitStretch {
			t.Errorf("ClassifyTitle(%q) = %+v, want FitStretch", title, cls)
		}
	}
}

func TestClassifyTitle_StretchRejectedWhenDisabled(t *testing.T) {
	title := "Principal DevOps Engineer"
	cls := ClassifyTitle(title, targetTrackRoles, false, false)
	if cls.Fit != FitReject || cls.Reason != ReasonSeniorityOutsideTarget {
		t.Errorf("ClassifyTitle(%q, allowStretch=false) = %+v, want reject/%s", title, cls, ReasonSeniorityOutsideTarget)
	}
}

func TestClassifyTitle_ManagementRejectedByDefault(t *testing.T) {
	titles := []string{
		"Director of DevOps",
		"Senior Director, Platform Engineering",
		"VP of Infrastructure",
		"Head of DevOps",
		"Engineering Manager, Platform",
		"Manager of Site Reliability Engineering",
		"Principal Product Manager, Platform",
		"Director, AI Infrastructure",
		"Director of Cloud Operations",
	}
	for _, title := range titles {
		cls := ClassifyTitle(title, targetTrackRoles, false, true)
		if cls.Fit != FitReject || cls.Reason != ReasonManagementTrackExcluded {
			t.Errorf("ClassifyTitle(%q) = %+v, want reject/%s", title, cls, ReasonManagementTrackExcluded)
		}
		if TitleEligibleForRoles(title, targetTrackRoles, false, true) {
			t.Errorf("TitleEligibleForRoles(%q) = true, want false (management track)", title)
		}
	}
}

func TestClassifyTitle_ManagementAllowedWhenOptedIn(t *testing.T) {
	title := "Director of DevOps"
	cls := ClassifyTitle(title, targetTrackRoles, true, true)
	if cls.Fit == FitReject {
		t.Errorf("ClassifyTitle(%q, allowManagement=true) = %+v, want acceptance", title, cls)
	}
}

func TestClassifyTitle_RoleTrackMismatchRejected(t *testing.T) {
	titles := []string{
		"Solutions Architect",
		"Technical Director", // also management, but should reject either way
	}
	for _, title := range titles {
		if TitleEligibleForRoles(title, targetTrackRoles, false, true) {
			t.Errorf("TitleEligibleForRoles(%q) = true, want false", title)
		}
	}
}

// TestClassifyTitle_AmbiguousDocumented pins down this policy's explicit
// decisions for titles the task instructions call out as ambiguous, so a
// future change to the classifier has to consciously revisit them rather
// than silently drifting.
func TestClassifyTitle_AmbiguousDocumented(t *testing.T) {
	cases := []struct {
		title      string
		wantReject bool
		note       string
	}{
		// "Manager" is treated as a management-track word even when it
		// modifies an engineering track, exactly like "Director of DevOps"
		// -- the primary noun is the management role, not the track.
		{"DevOps Manager", true, "manager-suffixed title treated as management track"},
		// "Architect" is an IC title; Platform is a target track word, so
		// this is accepted (as Adjacent, not Primary/Stretch).
		{"Platform Architect", false, "Architect is IC, Platform is a target track"},
		// No target-track keyword at all: rejected on plain role mismatch,
		// not on management grounds.
		{"Solutions Architect", true, "no DevOps/Platform/SRE/Infra/Cloud keyword present"},
		// Consultant is IC engagement, not management; DevOps track word
		// present.
		{"DevOps Consultant", false, "Consultant is IC, DevOps is a target track"},
		// Already a configured role in profile.yaml; IC support role.
		{"Production Support Engineer", false, "explicit target-adjacent role"},
		// "Director" makes this management track even though "Technical"
		// gives no track signal.
		{"Technical Director", true, "Director is management track"},
	}
	for _, c := range cases {
		got := !TitleEligibleForRoles(c.title, targetTrackRoles, false, true)
		if got != c.wantReject {
			t.Errorf("%s: TitleEligibleForRoles(%q) rejected=%v, want %v", c.note, c.title, got, c.wantReject)
		}
	}
}

func TestTitleEligible_UsesProductDefaults(t *testing.T) {
	// TitleEligible (the pre-existing exported function every call site but
	// ScreenJob still uses) must apply the product default: management
	// excluded, stretch allowed.
	if TitleEligible("Director of DevOps", targetTrackRoles) {
		t.Error("TitleEligible should reject management-track titles by default")
	}
	if !TitleEligible("Principal DevOps Engineer", targetTrackRoles) {
		t.Error("TitleEligible should accept Staff/Principal IC titles by default")
	}
	if !TitleEligible("Senior DevOps Engineer", targetTrackRoles) {
		t.Error("TitleEligible should still accept an ordinary primary match")
	}
}

func TestScreenJob_ManagementTrackReason(t *testing.T) {
	profile := &Profile{Roles: targetTrackRoles, RemoteOnly: true}
	result := ScreenJob(JobEligibilityInput{
		Title: "Director of DevOps", Location: "Remote", RemoteClaimed: true,
	}, profile)
	if result.Eligible || result.Code != ReasonManagementTrackExcluded {
		t.Fatalf("ScreenJob(Director of DevOps) = %+v, want ineligible/%s", result, ReasonManagementTrackExcluded)
	}
}

func TestScreenJob_ManagementRolesAllowedOptIn(t *testing.T) {
	profile := &Profile{Roles: targetTrackRoles, RemoteOnly: true, AllowManagementRoles: true}
	result := ScreenJob(JobEligibilityInput{
		Title: "Director of DevOps", Location: "Remote", RemoteClaimed: true,
	}, profile)
	if !result.Eligible {
		t.Fatalf("ScreenJob(Director of DevOps, AllowManagementRoles=true) = %+v, want eligible", result)
	}
}

// realProfileRoles mirrors profile.yaml's actual (2026-08-17 re-targeted)
// role list closely enough to reproduce bugs.md #557 against the roles that
// actually caused it in production, rather than only the smaller
// targetTrackRoles fixture above.
var realProfileRoles = []string{
	"DevOps Engineer", "Senior DevOps Engineer", "Azure DevOps Engineer",
	"AWS DevOps Engineer", "Cloud DevOps Engineer", "DevSecOps Engineer",
	"Senior DevSecOps Engineer", "DevOps Automation Engineer", "DevOps Specialist",
	"AI DevOps Engineer", "Staff DevOps Engineer", "Principal DevOps Engineer",
	"Platform Engineer", "Senior Platform Engineer", "Cloud Platform Engineer",
	"Developer Platform Engineer", "Platform Automation Engineer",
	"Platform Reliability Engineer", "Staff Platform Engineer", "Principal Platform Engineer",
	"Site Reliability Engineer", "Senior Site Reliability Engineer", "SRE", "Senior SRE",
	"SRE Engineer", "Staff Site Reliability Engineer", "Principal Site Reliability Engineer",
	"Production Engineer", "Production Support Engineer", "Infrastructure Automation Engineer",
	"Infrastructure Engineer", "Senior Infrastructure Engineer", "Cloud Infrastructure Engineer",
	"Cloud Engineer", "Senior Cloud Engineer", "Cloud Operations Engineer",
	"Cloud Systems Administrator", "Network Automation Engineer", "Observability Engineer",
	"AI Infrastructure Engineer", "AI Platform Engineer", "AIOps Engineer", "MLOps Engineer",
	"AI Automation Engineer",
}

// TestClassifyTitle_GenericSharedWordFalsePositives pins down bugs.md #557:
// these are real off-track titles pulled from a live discovery run that were
// wrongly admitted before the fix, purely because they share one generic
// word (systems/operations/support/network/production/cloud/platform) with
// a configured role and distinctiveRoleWords treated any single shared word
// as sufficient. None of them name a genuine DevOps/Platform/SRE/
// Infrastructure/Cloud engineering track.
func TestClassifyTitle_GenericSharedWordFalsePositives(t *testing.T) {
	titles := []string{
		"Senior Business Systems Analyst, Merchandising Systems",
		"Sr. Strategy & Operations Analyst, Deal Desk",
		"Senior Technical Customer Support Engineer",
		"Senior Operations Specialist",
		"GTM Operations Lead",
		"Product Specialist: Platform",
		"Technical Recruiter — Production Engineering",
		"GTM Business Operations Analyst",
		"Cloud Support Administrator",
	}
	for _, title := range titles {
		cls := ClassifyTitle(title, realProfileRoles, false, true)
		if cls.Fit != FitReject {
			t.Errorf("ClassifyTitle(%q) = %+v, want FitReject (off-track title admitted via a single generic shared word)", title, cls)
		}
	}
}

// TestClassifyTitle_GenericSharedWordFix_PreservesRealMatches confirms the
// #557 fix does not turn any of profile.yaml's actual must-accept titles
// into false negatives. Most of these are already literal role phrases
// (unaffected by the fallback change); a few genuinely rely on the
// single-distinctive-word fallback (e.g. "Platform Architect",
// "DevOps Consultant", "Network Reliability Engineer" via "reliability")
// and must keep working.
func TestClassifyTitle_GenericSharedWordFix_PreservesRealMatches(t *testing.T) {
	titles := []string{
		"Senior DevOps Engineer", "DevOps Engineer", "Azure DevOps Engineer",
		"DevSecOps Engineer", "Platform Engineer", "Senior Platform Engineer",
		"Cloud Platform Engineer", "Platform Reliability Engineer",
		"Site Reliability Engineer", "Senior Site Reliability Engineer", "SRE",
		"Infrastructure Engineer", "Senior Infrastructure Engineer",
		"Cloud Infrastructure Engineer", "Infrastructure Automation Engineer",
		"Production Engineer", "Production Support Engineer", "Observability Engineer",
		"Cloud Operations Engineer",
		// Fallback-dependent, must still be admitted:
		"Platform Architect",
		"DevOps Consultant",
		"Network Reliability Engineer",
	}
	for _, title := range titles {
		cls := ClassifyTitle(title, realProfileRoles, false, true)
		if cls.Fit == FitReject {
			t.Errorf("ClassifyTitle(%q) = reject (%s), want primary/adjacent/stretch acceptance", title, cls.Reason)
		}
	}
}

func TestScreenJob_RejectStretchSeniority(t *testing.T) {
	profile := &Profile{Roles: targetTrackRoles, RemoteOnly: true, RejectStretchSeniority: true}
	result := ScreenJob(JobEligibilityInput{
		Title: "Principal DevOps Engineer", Location: "Remote", RemoteClaimed: true,
	}, profile)
	if result.Eligible || result.Code != ReasonSeniorityOutsideTarget {
		t.Fatalf("ScreenJob(Principal DevOps Engineer, RejectStretchSeniority=true) = %+v, want ineligible/%s", result, ReasonSeniorityOutsideTarget)
	}
}
