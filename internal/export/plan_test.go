package export

import (
	"strings"
	"testing"
	"time"
)

func sampleJob() Job {
	return Job{JobID: "job-" + strings.Repeat("a", 64), SourceTitle: "Quarterly / Report.pdf", Submitter: "Smith/J", AcceptedAt: time.Date(2026, 10, 24, 3, 4, 5, 0, time.UTC), MediaType: "application/pdf"}
}

func TestDefaultStableAndRelative(t *testing.T) {
	a, err := MakePlan(Defaults{}, Overrides{}, sampleJob())
	if err != nil {
		t.Fatal(err)
	}
	b, err := MakePlan(Defaults{}, Overrides{}, sampleJob())
	if err != nil || a != b {
		t.Fatalf("not deterministic: %#v %#v %v", a, b, err)
	}
	if a.Folder != "." || !strings.HasPrefix(a.Filename, "Quarterly-Report-20261024-030405-job-") || !strings.HasSuffix(a.Filename, ".pdf") || strings.Contains(a.RelativePath, "/") {
		t.Fatalf("bad output: %#v", a)
	}
}

func TestDefaultAndAuthenticatedOverride(t *testing.T) {
	a, err := MakePlan(Defaults{Folder: "Prints/2026", FilenameTemplate: "{submitter}-{job_id}"}, Overrides{Folder: "Shared/Finance", FilenameTemplate: "{job_name}"}, sampleJob())
	if err != nil {
		t.Fatal(err)
	}
	if a.RelativePath != "Shared/Finance/Quarterly-Report.pdf" {
		t.Fatalf("bad path %q", a.RelativePath)
	}
}

func TestInvalidFolders(t *testing.T) {
	for _, folder := range []string{"/absolute", "../outside", "foo/../outside", "foo//bar", "foo/./bar", `C:\evil`, "shared\\folder", "CON", "COM1/file", "test/.hidden", "trailing. ", "file|name", "with\nnewline", "a/b/c/d/e/f/g"} {
		_, err := MakePlan(Defaults{Folder: folder}, Overrides{}, sampleJob())
		if err == nil {
			t.Errorf("accepted bad folder %q", folder)
		}
	}
}

func TestInvalidTemplates(t *testing.T) {
	for _, pattern := range []string{"../evil", "/absolute", `C:\evil`, "{unexpected}", "{job_name", "file\r\nheader", "CON", "NUL.txt", "trailing.", ".hidden", strings.Repeat("x", 181)} {
		_, err := MakePlan(Defaults{FilenameTemplate: pattern}, Overrides{}, sampleJob())
		if err == nil {
			t.Errorf("accepted bad template %q", pattern)
		}
	}
}

func TestReservedAndOddTitlesCannotEscape(t *testing.T) {
	job := sampleJob()
	job.SourceTitle = `..\\/\\evil\n`
	output, err := MakePlan(Defaults{}, Overrides{}, job)
	if err != nil || strings.ContainsAny(output.Filename, `/\\`) {
		t.Fatalf("unsafe output %q: %v", output.Filename, err)
	}
}

func TestMediaExtensions(t *testing.T) {
	cases := map[string]string{"application/pdf; charset=binary": ".pdf", "image/urf": ".urf", "image/pwg-raster": ".pwg", "application/postscript": ".ps", "text/plain": ".txt", "image/png": ".png", "image/jpeg": ".jpg", "unknown/type": ".bin"}
	for media, extension := range cases {
		j := sampleJob()
		j.MediaType = media
		a, err := MakePlan(Defaults{FilenameTemplate: "document"}, Overrides{}, j)
		if err != nil || !strings.HasSuffix(a.Filename, extension) {
			t.Errorf("%s: %q %v", media, a.Filename, err)
		}
	}
}

func TestInvalidIdentity(t *testing.T) {
	j := sampleJob()
	j.JobID = "bad"
	if _, err := MakePlan(Defaults{}, Overrides{}, j); err == nil {
		t.Fatal("accepted bad identity")
	}
	j = sampleJob()
	j.AcceptedAt = time.Time{}
	if _, err := MakePlan(Defaults{}, Overrides{}, j); err == nil {
		t.Fatal("accepted missing time")
	}
}
