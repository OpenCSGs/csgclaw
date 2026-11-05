package apitypes

type ListResponse[T any] struct {
	Items []T `json:"items"`
}
