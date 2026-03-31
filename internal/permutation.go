// Package internal provides permutation utilities for VoidBus v2.0.
package internal

import "crypto/rand"

// GeneratePermutations generates all possible permutations of codes with given depth.
// Allows repetition (same code can appear multiple times in chain).
// Used for codec chain hash matching on receive side.
func GeneratePermutations(codes []string, depth int) [][]string {
	if len(codes) == 0 || depth <= 0 {
		return nil
	}

	// Calculate total permutations: n^d
	total := powInt(len(codes), depth)
	result := make([][]string, 0, total)

	// Use iterative approach for better performance
	current := make([]int, depth)
	for i := 0; i < total; i++ {
		// Generate one permutation
		perm := make([]string, depth)
		for j := 0; j < depth; j++ {
			perm[j] = codes[current[j]]
		}
		result = append(result, perm)

		// Increment counter (like counting in base-n)
		for j := depth - 1; j >= 0; j-- {
			current[j]++
			if current[j] < len(codes) {
				break
			}
			current[j] = 0
		}
	}

	return result
}

// GenerateCombinations generates all combinations (no repetition) of codes.
// Each code can only appear once in the chain.
func GenerateCombinations(codes []string, depth int) [][]string {
	if len(codes) == 0 || depth <= 0 || depth > len(codes) {
		return nil
	}

	result := make([][]string, 0)
	generateCombinationsRecursive(codes, depth, 0, []string{}, &result)
	return result
}

func generateCombinationsRecursive(codes []string, depth, start int, current []string, result *[][]string) {
	if len(current) == depth {
		*result = append(*result, append([]string{}, current...))
		return
	}

	for i := start; i < len(codes); i++ {
		generateCombinationsRecursive(codes, depth, i+1, append(current, codes[i]), result)
	}
}

// powInt calculates base^exp for integers.
func powInt(base, exp int) int {
	result := 1
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

// RandomPermutation generates a random permutation of given codes and depth.
// Used on send side for codec chain selection.
func RandomPermutation(codes []string, depth int) []string {
	if len(codes) == 0 || depth <= 0 {
		return nil
	}

	result := make([]string, depth)
	for i := 0; i < depth; i++ {
		// Random select (allows repetition)
		idx := RandomInt(len(codes))
		result[i] = codes[idx]
	}
	return result
}

// RandomInt generates random integer in range [0, n).
func RandomInt(n int) int {
	if n <= 0 {
		return 0
	}
	b := make([]byte, 4)
	_, _ = randRead(b)
	val := int(b[0]) | int(b[1])<<8 | int(b[2])<<16 | int(b[3])<<24
	if val < 0 {
		val = -val
	}
	return val % n
}

// randRead is a helper to read random bytes (avoids crypto/rand import in every call)
func randRead(b []byte) (int, error) {
	return rand.Read(b)
}
