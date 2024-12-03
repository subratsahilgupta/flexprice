package types

type Filter struct {
	Limit  int    `form:"limit,default=10"`
	Offset int    `form:"offset,default=0"`
	Search string `form:"search"`
	Sort   string `form:"sort,default=created_at"`
	Order  string `form:"order,default=desc"`
}
