package auth

import "testing"

func TestGenerateTokenIsUniqueAndHashesConsistently(t *testing.T) {
	tokenA, hashA, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	tokenB, hashB, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	if tokenA == tokenB {
		t.Fatal("two generated tokens were identical")
	}
	if hashA == hashB {
		t.Fatal("two generated tokens hashed to the same value")
	}
	if HashToken(tokenA) != hashA {
		t.Error("HashToken(tokenA) does not match the hash GenerateToken returned")
	}
}
