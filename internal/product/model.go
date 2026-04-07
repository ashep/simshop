package product

type Product struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CreateRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CreateResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
