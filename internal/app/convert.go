package app

import "github.com/Kameleon21/oku/internal/api"

// apiUserBookAlias is a type alias so we can use api types without
// leaking them into the app package's public API.
type apiUserBookAlias = api.APIUserBook
