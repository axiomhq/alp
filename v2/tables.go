package alp

// Power-of-10 tables from the ALP paper.
// These are used for encoding and decoding float64 values.

const (
	numExp  = 18 // number of exponent values
	numFact = 18 // number of factor values
)

// expArr[i] = 10^i for encoding: value × 10^exp
var expArr = [numExp]float64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17,
}

// fracArr[i] = 10^-i for encoding: value × 10^-factor
var fracArr = [numExp]float64{
	1e-0, 1e-1, 1e-2, 1e-3, 1e-4, 1e-5, 1e-6, 1e-7, 1e-8, 1e-9,
	1e-10, 1e-11, 1e-12, 1e-13, 1e-14, 1e-15, 1e-16, 1e-17,
}

// factArr[i] = 10^i for decoding: encoded × 10^factor
var factArr = [numFact]uint64{
	1, 10, 100, 1000, 10000, 100000, 1000000, 10000000, 100000000, 1000000000,
	10000000000, 100000000000, 1000000000000, 10000000000000, 100000000000000,
	1000000000000000, 10000000000000000, 100000000000000000,
}
