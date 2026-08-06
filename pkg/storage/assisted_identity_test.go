package storage

import "testing"

func TestAssistedRequisitionID_ParsesRealATSPostingsAndRefusesSlugs(t *testing.T) {
	for _, tc := range []struct{ name, url, want string }{
		// The four Veeva postings from bug #521 differ only in this number.
		{"greenhouse board", "https://boards.greenhouse.io/veeva/jobs/293750", "293750"},
		{"greenhouse sibling", "https://boards.greenhouse.io/veeva/jobs/293752", "293752"},
		{"greenhouse embedded", "https://veeva.com/careers?gh_jid=293750", "293750"},
		{"lever posting", "https://jobs.lever.co/veeva/1f2e3d4c-5b6a-7890-abcd-ef0123456789", "1f2e3d4c-5b6a-7890-abcd-ef0123456789"},
		{"lever apply form", "https://jobs.lever.co/veeva/1f2e3d4c-5b6a-7890-abcd-ef0123456789/apply", "1f2e3d4c-5b6a-7890-abcd-ef0123456789"},
		{"workday requisition", "https://acme.wd1.myworkdayjobs.com/en-US/careers/job/Remote/Engineer_JR-12345", "Engineer_JR-12345"},
		{"trailing slash", "https://boards.greenhouse.io/veeva/jobs/293750/", "293750"},
		// The remaining cases come from URLs actually sitting in the live
		// assisted queue, each of which the first version of this parser
		// silently gave up on.
		{"jazzhr id before slug", "https://holafly.applytojob.com/apply/7O47y8kouq/Senior-Backend-Engineer-PythonDjango", "7O47y8kouq"},
		{"employer site id before slug", "https://careers.southstatebank.com/us/en/job/R-05240/Payment-Platform-DevOps-Engineer-Charleston-SC", "R-05240"},
		{"workday slug carrying its requisition", "https://tbkbank.wd1.myworkdayjobs.com/tfin/job/remote---united-states/senior-devops-engineer-aws--kubernetes--cloud-infrastructure---remote_req-4917", "req-4917"},
		{"jobvite opaque token", "https://jobs.jobvite.com/relatient/job/orrtAfwF", "orrtAfwF"},
		// A slug cannot be told apart from another posting's slug and must not
		// be presented as if it could be, and neither may a capitalised path
		// word that happens to sit where an identifier would.
		{"role slug", "https://careers.example.com/roles/senior-software-engineer", ""},
		{"capitalised path word", "https://workday.wd5.myworkdayjobs.com/Workday/?source=Careers_Website_oo", ""},
		{"board listing page", "https://jobs.jobvite.com/relatient/jobs", ""},
		{"employer root", "https://jobs.lever.co/veeva", ""},
		{"no scheme", "boards.greenhouse.io/veeva/jobs/293750", ""},
		{"non http scheme", "file:///etc/passwd", ""},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AssistedRequisitionID(tc.url); got != tc.want {
				t.Fatalf("AssistedRequisitionID(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestMarkAssistedDuplicates_CountsSiblingsBySharedCompanyAndRole(t *testing.T) {
	jobs := []AssistedJob{
		{ID: "1", Company: "Veeva Systems Inc", Role: "Senior Software Engineer - Python", RequisitionID: "293750"},
		{ID: "2", Company: "Veeva Systems", Role: "Senior Software Engineer, Python", RequisitionID: "293752"},
		{ID: "3", Company: "Veeva Systems", Role: "Senior Data Engineer"},
		{ID: "4", Company: "Other Co", Role: "Senior Software Engineer - Python"},
		{ID: "5", Company: "", Role: "Senior Software Engineer - Python"},
	}
	markAssistedDuplicates(jobs)
	want := map[string]int{"1": 1, "2": 1, "3": 0, "4": 0, "5": 0}
	for _, job := range jobs {
		if job.DuplicateSiblings != want[job.ID] {
			t.Fatalf("job %s siblings = %d, want %d", job.ID, job.DuplicateSiblings, want[job.ID])
		}
		if job.Ambiguous {
			t.Fatalf("job %s reported ambiguous despite a unique requisition", job.ID)
		}
	}
}

// A distinguisher an operator checks and finds matching is worse than none, so
// duplicates that carry nothing distinct -- or the same thing -- must say so.
func TestMarkAssistedDuplicates_FlagsSiblingsItCannotTellApart(t *testing.T) {
	jobs := []AssistedJob{
		{ID: "1", Company: "Acme", Role: "Backend Engineer"},
		{ID: "2", Company: "Acme", Role: "Backend Engineer"},
		{ID: "3", Company: "Globex", Role: "Backend Engineer", Location: "Remote, US"},
		{ID: "4", Company: "Globex", Role: "Backend Engineer", Location: "Remote, US"},
		{ID: "5", Company: "Initech", Role: "Backend Engineer", Location: "Remote, US", RequisitionID: "11"},
		{ID: "6", Company: "Initech", Role: "Backend Engineer", Location: "Remote, US", RequisitionID: "12"},
	}
	markAssistedDuplicates(jobs)
	want := map[string]bool{"1": true, "2": true, "3": true, "4": true, "5": false, "6": false}
	for _, job := range jobs {
		if job.DuplicateSiblings != 1 {
			t.Fatalf("job %s siblings = %d, want 1", job.ID, job.DuplicateSiblings)
		}
		if job.Ambiguous != want[job.ID] {
			t.Fatalf("job %s ambiguous = %v, want %v", job.ID, job.Ambiguous, want[job.ID])
		}
	}
}
