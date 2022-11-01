package vz

func Available(version float64) bool {
	return macOSAvailable(version) != nil
}
