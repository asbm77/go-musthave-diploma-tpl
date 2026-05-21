// internal/utils/luhn.go
package utils

// ValidateLuhn проверяет номер заказа по алгоритму Луна
func ValidateLuhn(number string) bool {
	var sum int
	var alternate bool

	// Проходим по цифрам справа налево
	for i := len(number) - 1; i >= 0; i-- {
		digit := int(number[i] - '0')

		if digit < 0 || digit > 9 {
			return false
		}

		if alternate {
			digit *= 2
			if digit > 9 {
				digit = digit%10 + 1
			}
		}

		sum += digit
		alternate = !alternate
	}

	return sum%10 == 0
}
