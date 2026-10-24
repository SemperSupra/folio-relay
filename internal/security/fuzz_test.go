package security

import "testing"

func FuzzAdmissionBounds(f *testing.F) {
	f.Add(int64(1), int64(1), int64(1), int64(1), int64(1))
	f.Add(int64(1024), int64(100), int64(2), int64(2048), int64(3))
	f.Add(int64(1), int64(1_000_000), int64(1), int64(1), int64(1))

	limits := AdmissionLimits{
		MaxBytes:         100 << 20,
		MaxPages:         10000,
		MaxCopies:        1000,
		MaxExpandedBytes: 1 << 30,
		MaxFanout:        16,
	}

	f.Fuzz(func(t *testing.T, bytes, pages, copies, expanded, fanout int64) {
		req := AdmissionRequest{
			Bytes: bytes, Pages: pages, Copies: copies,
			ExpandedBytes: expanded, Fanout: fanout,
		}
		err := ValidateAdmission(req, limits)

		if bytes < 0 || pages < 0 || copies < 0 || expanded < 0 || fanout < 0 {
			if err == nil {
				t.Fatalf("negative input unexpectedly accepted: %+v", req)
			}
			return
		}
		if bytes > limits.MaxBytes ||
			pages > limits.MaxPages ||
			copies > limits.MaxCopies ||
			expanded > limits.MaxExpandedBytes ||
			fanout > limits.MaxFanout {
			if err == nil {
				t.Fatalf("over-limit input unexpectedly accepted: %+v", req)
			}
		}
	})
}
