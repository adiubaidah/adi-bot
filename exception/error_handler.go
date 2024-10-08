package exception

import (
	"fmt"
	"net/http"

	"github.com/adiubaidah/wasabi/helper"
	"github.com/adiubaidah/wasabi/model"

	"github.com/go-playground/validator/v10"
)

func ErrorHandler(writer http.ResponseWriter, request *http.Request, err any) {

	if notFoundError(writer, request, err) {
		return
	}

	if validationErrors(writer, request, err) {
		return
	}

	if notAuthorizedError(writer, request, err) {
		return
	}

	if forbiddenError(writer, request, err) {
		return
	}

	// Log the error for debugging
	fmt.Println("Unhandled error:", err)
	internalServerError(writer, request, err)
}

func validationErrors(writer http.ResponseWriter, _ *http.Request, err any) bool {
	exception, ok := err.(validator.ValidationErrors) //if convertion is success, ok will be true
	if ok {
		writer.Header().Set("Content-Type", "application/json")

		webResponse := &model.WebResponse{
			Code:   http.StatusBadRequest,
			Status: "BAD REQUEST",
			Data:   exception.Error(),
		}

		helper.WriteToResponseBody(writer, webResponse)
		return true
	} else {
		return false
	}
}

func notFoundError(writer http.ResponseWriter, _ *http.Request, err any) bool {
	exception, ok := err.(NotFoundError)
	if ok {
		writer.Header().Set("Content-Type", "application/json")

		webResponse := &model.WebResponse{
			Code:   http.StatusNotFound,
			Status: "NOT FOUND",
			Data:   exception.Error,
		}

		helper.WriteToResponseBody(writer, webResponse)
		return true
	} else {
		return false
	}
}

func notAuthorizedError(writer http.ResponseWriter, _ *http.Request, err any) bool {
	exception, ok := err.(UnauthorizedError)
	if ok {
		writer.Header().Set("Content-Type", "application/json")

		webResponse := &model.WebResponse{
			Code:   http.StatusUnauthorized,
			Status: "UNAUTHORIZED",
			Data:   exception.Error,
		}

		helper.WriteToResponseBody(writer, webResponse)
		return false
	} else {
		return true
	}
}

func forbiddenError(writer http.ResponseWriter, _ *http.Request, err any) bool {
	exception, ok := err.(ForbiddenError)
	if ok {
		writer.Header().Set("Content-Type", "application/json")

		webResponse := &model.WebResponse{
			Code:   http.StatusForbidden,
			Status: "FORBIDDEN",
			Data:   exception.Error,
		}

		helper.WriteToResponseBody(writer, webResponse)
		return false
	} else {
		return true
	}
}

func internalServerError(writer http.ResponseWriter, _ *http.Request, err any) {
	writer.Header().Set("Content-Type", "application/json")
	webResponse := &model.WebResponse{
		Code:   http.StatusInternalServerError,
		Status: "INTERNAL SERVER ERROR",
		Data:   err,
	}

	helper.WriteToResponseBody(writer, webResponse)
}
