package main

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/AdventurerAmer/movie-reservation-system/internal"
)

type CreateMovieResponse struct {
	Movie *internal.Movie `json:"movie"`
}

// createMovieHandler godoc
//
//	@Summary		Creates a movie
//	@Description	sreates a movie
//	@Tags			movies
//	@Accept			json
//	@Produce		json
//	@Param			title	body		string	true	"title"
//	@Param			runtime	body		int		true	"runtime"
//	@Param			year	body		int		true	"year"
//	@Param			genres	body		array	true	"genres"
//	@Success		201		{object}	CreateMovieResponse
//	@Failure		400		{object}	ViolationsMessage
//	@Failure		500		{object}	ResponseError
//	@Router			/movies [post]
func (app *Application) createMovieHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title   string   `json:"title"`
		Runtime int32    `json:"runtime"`
		Year    int32    `json:"year"`
		Genres  []string `json:"genres"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	v.Check(req.Title != "", "title", "must be provided")
	v.Check(req.Runtime > 0, "runtime", "must be greater than zero")
	v.Check(req.Year > 0, "year", "must be greater than zero")
	v.Check(len(req.Genres) != 0, "genres", "must be provided")

	for idx, g := range req.Genres {
		v.Check(g != "", fmt.Sprintf("genre at index: %d", idx), "must be provided")
	}

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	m, err := app.storage.Movies.Create(req.Title, req.Runtime, req.Year, req.Genres)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	writeJSON(CreateMovieResponse{Movie: m}, http.StatusCreated, w)
}

type GetMovieResponse struct {
	Movie *internal.Movie `json:"movie"`
}

// getMovieHandler godoc
//
//	@Summary		Gets a movie
//	@Description	gets a movie by id
//	@Tags			movies
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"id"
//	@Success		200	{object}	GetMovieResponse
//	@Failure		400	{object}	ResponseMessage
//	@Failure		404	{object}	ResponseMessage
//	@Failure		500	{object}	ResponseError
//	@Router			/movies/{id} [get]
func (app *Application) getMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	m, err := app.storage.Movies.GetByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if m == nil {
		writeNotFound(w)
		return
	}
	writeJSON(GetMovieResponse{Movie: m}, http.StatusOK, w)
}

type GetMoviesResponse struct {
	Movies   []internal.Movie   `json:"movies"`
	MetaData *internal.MetaData `json:"meta_data"`
}

// getMoviesHandler godoc
//
//	@Summary		Gets a list of movies
//	@Description	gets a list movies with search paramters
//	@Tags			movies
//	@Accept			json
//	@Produce		json
//	@Param			title		query	string	false	"title"
//	@Param			genres		query	string	false	"genres comma separated"
//	@Param			page		query	int		false	"page number"
//	@Param			page_size	query	int		false	"number of pages"
//	@param			sort		query	string	false	"sort params (id, title, year, runtime) prefix with - to sort descending"

// @Success	200	{object}	GetMoviesResponse
// @Failure	400	{object}	ViolationsMessage
// @Failure	500	{object}	ResponseError
// @Router		/movies [get]
func (app *Application) getMoviesHandler(w http.ResponseWriter, r *http.Request) {
	v := NewValidator()

	title := getQueryStringOr(r, "title", "")
	genres := getQueryCSVOr(r, "genres", []string{})
	page := getQueryIntOr(r, "page", 1, v)
	pageSize := getQueryIntOr(r, "page_size", 20, v)
	sort := getQueryStringOr(r, "sort", "id")

	v.Check(page > 0 && page <= 10_000_000, "page", "must be between 1 and 10_000_000")
	v.Check(pageSize > 0 && pageSize <= 100, "page_size", "must be between 1 and 100")

	sortList := []string{"id", "-id", "title", "-title", "year", "-year", "runtime", "-runtime"}
	v.Check(slices.Contains(sortList, sort), fmt.Sprintf("sort-%s", sort), "not supported")

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	movies, metaData, err := app.storage.Movies.GetAll(title, genres, page, pageSize, sort)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	writeJSON(GetMoviesResponse{Movies: movies, MetaData: metaData}, http.StatusOK, w)
}

type UpdateMovieResponse struct {
	Movie internal.Movie `json:"movie"`
}

// updateMovieHandler godoc
//
//	@Summary		Updates a movie
//	@Description	updates a movie by id
//	@Tags			movies
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int		true	"id"
//	@Param			title	body	string	false	"title"
//	@Param			runtime	body	int		false	"runtime"
//	@Param			year	body	int		false	"year"
//	@Param			genres	body	array	false	"genres comma separated"

// @Success	200	{object}	UpdateMovieResponse
// @Failure	400	{object}	ViolationsMessage
// @Failure	404	{object}	ResponseMessage
// @Failure	500	{object}	ResponseError
// @Router		/movies/{id} [put]
func (app *Application) updateMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Title   *string   `json:"title"`
		Runtime *int32    `json:"runtime"`
		Year    *int32    `json:"year"`
		Genres  *[]string `json:"genres"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	if req.Title != nil {
		v.Check(*req.Title != "", "title", "must be provided")
	}
	if req.Runtime != nil {
		v.Check(*req.Runtime > 0, "runtime", "must be greater than zero")
	}
	if req.Year != nil {
		v.Check(*req.Year > 0, "year", "must be greater than zero")
	}
	if req.Genres != nil {
		v.Check(len(*req.Genres) != 0, "genres", "must be provided")
		for idx, g := range *req.Genres {
			v.Check(g != "", fmt.Sprintf("genre at index: %d", idx), "must be provided")
		}
	}
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}
	m, err := app.storage.Movies.GetByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if m == nil {
		writeNotFound(w)
		return
	}
	if req.Title != nil {
		m.Title = *req.Title
	}
	if req.Runtime != nil {
		m.Runtime = *req.Runtime
	}
	if req.Year != nil {
		m.Year = *req.Year
	}
	if req.Genres != nil {
		m.Genres = *req.Genres
	}
	err = app.storage.Movies.Update(m)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"movie": m,
	}
	writeJSON(res, http.StatusOK, w)
}

// deleteMovieHandler godoc
//
//	@Summary		Deletes a movie
//	@Description	deletes a movie by id
//	@Tags			movies
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int		true	"title"
//	@Param			runtime	body	string	false	"genres comma separated"
//	@Param			year	body	string	false	"title"
//	@Param			genres	body	string	false	"genres comma separated"

// @Success	200	{object}	ResponseMessage
// @Failure	400	{object}	ViolationsMessage
// @Failure	500	{object}	ResponseError
// @Router		/movies/{id} [delete]
func (app *Application) deleteMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	m, err := app.storage.Movies.GetByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if m == nil {
		writeNotFound(w)
		return
	}
	err = app.storage.Movies.Delete(m)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	writeJSON(ResponseMessage{Message: "resource deleted successfully"}, http.StatusOK, w)
}
