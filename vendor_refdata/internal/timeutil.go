package internal

import "time"

func parseTime(layout, value string) (int64, error) {
	t, err := time.Parse(layout, value)
	if err != nil {
		return 0, err
	}
	return t.UTC().Unix(), nil
}
