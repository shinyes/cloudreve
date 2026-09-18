package manager

import "testing"

// TestPlanTransfer pins the encryption matrix of a relocation. Each combination has
// a different on-disk consequence, and getting one wrong either stores plaintext
// where ciphertext was promised or leaves a blob whose key material no longer matches
// it, which corrupts the file. The flags are also mutually exclusive by construction:
// asserting that here keeps a future edit from turning two modes on at once.
func TestPlanTransfer(t *testing.T) {
	cases := []struct {
		name           string
		encrypted      bool
		targetEncrypts bool
		want           transferPlan
	}{
		{
			name:           "encrypted onto encrypting policy carries the ciphertext",
			encrypted:      true,
			targetEncrypts: true,
			want:           transferPlan{rawCiphertext: true, reuseCiphertext: true},
		},
		{
			name:           "encrypted onto plain policy decrypts",
			encrypted:      true,
			targetEncrypts: false,
			want:           transferPlan{decrypting: true},
		},
		{
			name:           "plain onto encrypting policy encrypts on write",
			encrypted:      false,
			targetEncrypts: true,
			want:           transferPlan{encryptOnWrite: true},
		},
		{
			name:           "plain onto plain policy copies as is",
			encrypted:      false,
			targetEncrypts: false,
			want:           transferPlan{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := planTransfer(tc.encrypted, tc.targetEncrypts)

			if got != tc.want {
				t.Fatalf("planTransfer(%v, %v) = %+v, want %+v", tc.encrypted, tc.targetEncrypts, got, tc.want)
			}

			// Exactly one mode may be selected, and raw reads are only ever paired
			// with carrying the ciphertext over.
			modes := 0
			for _, on := range []bool{got.decrypting, got.reuseCiphertext, got.encryptOnWrite} {
				if on {
					modes++
				}
			}
			if modes > 1 {
				t.Errorf("more than one transfer mode selected: %+v", got)
			}
			if got.rawCiphertext && !got.reuseCiphertext {
				t.Errorf("raw ciphertext reads must imply reuse, got %+v", got)
			}
			if got.reuseCiphertext && got.rawCiphertext == false && tc.encrypted {
				t.Errorf("encrypted reuse without raw reads would decrypt then re-encrypt: %+v", got)
			}

			// The decision itself never carries key material; it is either reused from
			// the entity or generated while writing, both handled by the caller.
			if got.metadata != nil {
				t.Errorf("planTransfer must not fabricate metadata, got %+v", got.metadata)
			}
		})
	}
}

// TestPlanTransferMetadataIsNeverClearedByReuse documents why moving ciphertext onto
// another encrypting policy leaves the entity untouched: the per-blob key stays valid
// because the bytes are not rewritten.
func TestPlanTransferMetadataIsNeverClearedByReuse(t *testing.T) {
	plan := planTransfer(true, true)
	if plan.decrypting {
		t.Fatal("moving ciphertext onto an encrypting policy must not decrypt")
	}
	if plan.metadata != nil {
		t.Fatal("the plan must not replace existing key material")
	}
	if !plan.reuseCiphertext || !plan.rawCiphertext {
		t.Fatalf("expected verbatim ciphertext handling, got %+v", plan)
	}
}
