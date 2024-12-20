package types

const (
	DefaultFilterLimit  = 50
	DefaultFilterOffset = 0
)

type Filter struct {
	Limit  int    `form:"limit,default=50"`
	Offset int    `form:"offset,default=0"`
	Status Status `form:"status,default=published"`
	Sort   string `form:"sort,default=created_at"`
	Order  string `form:"order,default=desc"`
}

func GetDefaultFilter() Filter {
	return Filter{
		Limit:  DefaultFilterLimit,
		Offset: DefaultFilterOffset,
	}
}
