package main

import (
	"fmt"
	"net/http"
	"slices"
)

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
	res := map[string]any{
		"movie": m,
	}
	writeJSON(res, http.StatusCreated, w)
}

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
	res := map[string]any{
		"movie": m,
	}
	writeJSON(res, http.StatusOK, w)
}

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

	movies, meta, err := app.storage.Movies.GetAll(title, genres, page, pageSize, sort)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"movies": movies,
		"meta":   meta,
	}
	writeJSON(res, http.StatusOK, w)
}

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
	res := map[string]any{
		"message": "resource deleted successfully",
	}
	writeJSON(res, http.StatusOK, w)
}
