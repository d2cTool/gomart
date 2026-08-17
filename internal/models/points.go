// Package models содержит доменные типы и ошибки накопительной системы
// лояльности «Гофермарт».
package models

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// pointsScale — количество сотых долей балла в одном балле.
const pointsScale = 100

// Points представляет количество баллов лояльности во внутреннем формате —
// целое число сотых долей балла. Такое представление исключает ошибки
// округления, свойственные числам с плавающей точкой, при этом в JSON тип
// сериализуется как обычное десятичное число (например, 500.5).
type Points int64

// NewPoints переводит дробное количество баллов во внутреннее представление,
// округляя значение до сотых долей.
func NewPoints(v float64) Points {
	return Points(math.Round(v * pointsScale))
}

// Float64 возвращает количество баллов в виде числа с плавающей точкой.
func (p Points) Float64() float64 {
	return float64(p) / pointsScale
}

// String возвращает десятичное представление количества баллов.
func (p Points) String() string {
	return strconv.FormatFloat(p.Float64(), 'f', -1, 64)
}

// MarshalJSON сериализует баллы в JSON-число без лишних нулей в дробной части.
func (p Points) MarshalJSON() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalJSON разбирает JSON-число в баллы, округляя его до сотых долей.
func (p *Points) UnmarshalJSON(data []byte) error {
	var v json.Number
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("decode points: %w", err)
	}
	f, err := v.Float64()
	if err != nil {
		return fmt.Errorf("decode points value %q: %w", v.String(), err)
	}
	*p = NewPoints(f)
	return nil
}
