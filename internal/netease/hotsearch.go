package netease

// HotSearch is a keyword on NetEase's hot search list: the word itself
// and, when the list marks it, the badge it wears (hot, new, exploding).
type HotSearch struct {
	Word    string `json:"searchWord"`
	Score   int    `json:"score"`
	Content string `json:"content"`
	Icon    int    `json:"iconType"`
	IconURL string `json:"iconUrl"`
}

// HotSearches is NetEase's hot search list, the keywords trending now.
// The hot-search-list endpoint can refuse an anonymous client, in which
// case the plain search/hot list is asked instead.
func HotSearches() ([]HotSearch, error) {
	var res struct {
		Code int         `json:"code"`
		Data []HotSearch `json:"data"`
	}
	if err := get("/api/hotsearchlist/get", nil, &res); err == nil && len(res.Data) > 0 {
		return res.Data, nil
	}
	var alt struct {
		Code   int `json:"code"`
		Result struct {
			Hots []struct {
				First    string `json:"first"`
				IconType int    `json:"iconType"`
			} `json:"hots"`
		} `json:"result"`
	}
	if err := get("/api/search/hot", nil, &alt); err != nil {
		return nil, err
	}
	out := make([]HotSearch, 0, len(alt.Result.Hots))
	for _, h := range alt.Result.Hots {
		out = append(out, HotSearch{Word: h.First, Icon: h.IconType})
	}
	return out, nil
}
