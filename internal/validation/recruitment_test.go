package validation

import "testing"

func TestIsRecruitmentSlot(t *testing.T) {
	accepted := []string{
		"Leer", "Maintank", "Offtank", "Irgendein", "Irgendein Tank", "Irgendein Heiler", "Irgendein MDD", "Irgendein RDD",
		"Krieger Furor", "Paladin Heilig", "Priester Schatten", "Jäger Treffsicherheit", "Druide Wiederherstellung",
	}
	for _, value := range accepted {
		if !IsRecruitmentSlot(value) {
			t.Errorf("%q must be accepted", value)
		}
	}
	rejected := []string{"", " ", "<b>x</b>", "paladin heilig", " Paladin Heilig", "Paladin:Holy", "Krieger", "Irgendein Schurke", "Irgendein  MDD"}
	for _, value := range rejected {
		if IsRecruitmentSlot(value) {
			t.Errorf("%q must be rejected", value)
		}
	}
}
