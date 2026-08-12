package config

import "testing"

func remoteOnlyProfile(roles ...string) *Profile {
	if len(roles) == 0 {
		roles = []string{"Software Engineer", "DevOps Engineer", "Platform Engineer"}
	}
	return &Profile{RemoteOnly: true, Roles: roles}
}

func TestRemoteEligible_FullyRemote(t *testing.T) {
	ok, reason := RemoteEligible(true, "Remote - US", "This is a fully remote position with no office requirement.")
	if !ok {
		t.Fatalf("expected fully remote job to be eligible, got reason %q", reason)
	}
}

func TestRemoteEligible_Hybrid(t *testing.T) {
	ok, _ := RemoteEligible(true, "Hybrid - Austin, TX", "")
	if ok {
		t.Fatal("expected hybrid job to be rejected")
	}
}

func TestRemoteEligible_OnSite(t *testing.T) {
	ok, _ := RemoteEligible(false, "Onsite - Austin, TX", "This role is on-site five days a week.")
	if ok {
		t.Fatal("expected on-site job to be rejected")
	}
}

func TestRemoteEligible_MandatoryMonthlyOfficeAttendance(t *testing.T) {
	ok, _ := RemoteEligible(true, "Remote", "Remote role with mandatory monthly office attendance required.")
	if ok {
		t.Fatal("expected mandatory monthly office attendance to be rejected")
	}
}

func TestRemoteEligible_MandatoryQuarterlyOfficeAttendance(t *testing.T) {
	ok, _ := RemoteEligible(true, "Remote", "This remote role requires quarterly office attendance for planning weeks.")
	if ok {
		t.Fatal("expected mandatory quarterly office attendance to be rejected")
	}
}

func TestRemoteEligible_RemoteSlashHybrid(t *testing.T) {
	ok, _ := RemoteEligible(true, "Remote/Hybrid", "")
	if ok {
		t.Fatal("expected Remote/Hybrid to be rejected")
	}
}

func TestRemoteEligible_AmbiguousWorkplace(t *testing.T) {
	ok, _ := RemoteEligible(false, "Flexible workplace", "We offer a flexible workplace arrangement.")
	if ok {
		t.Fatal("expected ambiguous workplace arrangement to be rejected")
	}
}

func TestRemoteEligible_RemoteInTitleHybridInDescription(t *testing.T) {
	ok, _ := RemoteEligible(true, "Remote Software Engineer", "Note: this is a hybrid role requiring in-office days.")
	if ok {
		t.Fatal("expected remote-in-title-but-hybrid-in-description to be rejected")
	}
}

func TestRemoteEligible_RelocationRequired(t *testing.T) {
	ok, _ := RemoteEligible(true, "Remote (relocation required)", "")
	if ok {
		t.Fatal("expected a posting requiring relocation to be rejected")
	}
}

func TestRemoteEligible_CommutingDistance(t *testing.T) {
	ok, _ := RemoteEligible(true, "Remote", "Candidates must reside within commuting distance of our office in case attendance is required.")
	if ok {
		t.Fatal("expected a commuting-distance requirement to be rejected")
	}
}

func TestRemoteEligible_RemoteAfterOnboardingWithOfficeAttendance(t *testing.T) {
	ok, _ := RemoteEligible(true, "Remote after onboarding", "Onboarding requires significant office attendance for the first month.")
	if ok {
		t.Fatal("expected remote-after-onboarding-with-office-attendance to be rejected")
	}
}

func TestRemoteEligible_OfficeAttendanceAsNeeded(t *testing.T) {
	ok, _ := RemoteEligible(true, "Remote", "Office attendance as needed; this is a mandatory expectation for all staff.")
	if ok {
		t.Fatal("expected mandatory 'as needed' office attendance to be rejected")
	}
}

func TestRemoteEligible_NoEvidenceDefaultsToRejected(t *testing.T) {
	ok, reason := RemoteEligible(false, "", "")
	if ok {
		t.Fatalf("expected a posting with no remote evidence to be rejected, got reason %q", reason)
	}
}

func TestIsEligibleJob_HighScoringHybridStillRejected(t *testing.T) {
	// A high fit score is never part of IsEligibleJob's input at all: the
	// gate must run before scoring, and this test documents that omission is
	// intentional, not an oversight -- there is no parameter here a caller
	// could set to make a hybrid job pass.
	profile := remoteOnlyProfile("Software Engineer")
	ok, _ := IsEligibleJob(JobEligibilityInput{
		Title:         "Software Engineer",
		Location:      "Hybrid - NYC",
		RemoteClaimed: true,
	}, profile)
	if ok {
		t.Fatal("expected hybrid job to be rejected regardless of title/score attractiveness")
	}
}

func TestIsEligibleJob_PerfectTitleOnSiteStillRejected(t *testing.T) {
	profile := remoteOnlyProfile("Software Engineer")
	ok, _ := IsEligibleJob(JobEligibilityInput{
		Title:         "Software Engineer",
		Location:      "On-site - San Francisco, CA",
		RemoteClaimed: false,
	}, profile)
	if ok {
		t.Fatal("expected perfect-title on-site job to still be rejected")
	}
}

func TestTitleEligible_SoftwareEngineerAlwaysKept(t *testing.T) {
	roles := []string{"Software Engineer", "DevOps Engineer"}
	if !TitleEligible("Software Engineer", roles) {
		t.Fatal("Software Engineer must remain eligible")
	}
}

func TestTitleEligible_PlatformDutiesUnderGenericTitle(t *testing.T) {
	roles := []string{"Software Engineer", "DevOps Engineer", "Platform Engineer"}
	// A generic "Software Engineer" title is always retained (see
	// TestTitleEligible_SoftwareEngineerAlwaysKept); this documents that a
	// title carrying a distinctive platform/DevOps word also matches
	// directly, independent of the generic-title carve-out.
	if !TitleEligible("Software Engineer - Platform Infrastructure", roles) {
		t.Fatal("expected a platform/infrastructure Software Engineer posting to match")
	}
}

func TestTitleEligible_RemovedTitleNoLongerMatches(t *testing.T) {
	// "Data Engineer" was deliberately removed from the target profile.
	roles := []string{"Software Engineer", "DevOps Engineer", "Platform Engineer"}
	if TitleEligible("Data Engineer", roles) {
		t.Fatal("expected a removed title with no distinctive matching word to be ineligible")
	}
}

func TestIsEligibleJob_RemoteOnlyFalseSkipsRemoteGate(t *testing.T) {
	profile := &Profile{RemoteOnly: false, Roles: []string{"Software Engineer"}}
	ok, reason := IsEligibleJob(JobEligibilityInput{
		Title:         "Software Engineer",
		Location:      "Hybrid - NYC",
		RemoteClaimed: false,
	}, profile)
	if !ok {
		t.Fatalf("expected remote gate to be skipped when RemoteOnly is false, got reason %q", reason)
	}
}
