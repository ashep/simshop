package shop

type Shop struct {
	ID    string            `json:"id"`
	Names map[string]string `json:"names"`
}

type CreateResponse struct {
	ID string `json:"id"`
}
