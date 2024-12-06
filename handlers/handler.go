package handlers

import "github.com/labstack/echo/v4"

type handler interface {
	HandleGetMain(c echo.Context) error
	HandlePostProfile(c echo.Context) error
	HandlePostRoom(c echo.Context) error
}
