package utils

func GetUpdateColumns(updates map[string]interface{}) []string {
	columns := make([]string, 0, len(updates))
	for column := range updates {
		columns = append(columns, column)
	}
	return columns
}
