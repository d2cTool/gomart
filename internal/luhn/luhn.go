// Package luhn реализует проверку номеров заказов по алгоритму Луна.
package luhn

// IsValid сообщает, является ли строка корректным числом по алгоритму Луна.
// Пустая строка и строки, содержащие любые символы кроме десятичных цифр,
// считаются некорректными.
func IsValid(number string) bool {
	if number == "" {
		return false
	}

	sum := 0
	double := false
	for i := len(number) - 1; i >= 0; i-- {
		c := number[i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}

	return sum%10 == 0
}
