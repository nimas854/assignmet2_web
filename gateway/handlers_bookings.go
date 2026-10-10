package main

import (
	"net/http"

	bookingpb "trailbooking/gen/bookingpb"
)

// POST /bookings
func (g *Gateway) createBooking(w http.ResponseWriter, r *http.Request) {
	req := &bookingpb.CreateBookingRequest{}
	if !decodeProto(w, r, req) {
		return
	}
	ctx, cancel := g.ctx(r)
	defer cancel()

	booking, err := g.bookings.CreateBooking(ctx, req)
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	w.Header().Set("Location", "/bookings/"+booking.GetId())
	writeProto(w, http.StatusCreated, booking)
}

// GET /bookings
func (g *Gateway) listBookings(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := g.ctx(r)
	defer cancel()

	res, err := g.bookings.ListBookings(ctx, &bookingpb.ListBookingsRequest{})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeProto(w, http.StatusOK, res)
}

// GET /bookings/{id}
func (g *Gateway) getBooking(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := g.ctx(r)
	defer cancel()

	booking, err := g.bookings.GetBooking(ctx, &bookingpb.GetBookingRequest{Id: r.PathValue("id")})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeProto(w, http.StatusOK, booking)
}

// PUT /bookings/{id}
func (g *Gateway) updateBooking(w http.ResponseWriter, r *http.Request) {
	req := &bookingpb.UpdateBookingRequest{}
	if !decodeProto(w, r, req) {
		return
	}
	req.Id = r.PathValue("id") // the id always comes from the URL
	ctx, cancel := g.ctx(r)
	defer cancel()

	booking, err := g.bookings.UpdateBooking(ctx, req)
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeProto(w, http.StatusOK, booking)
}

// DELETE /bookings/{id}
func (g *Gateway) deleteBooking(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := g.ctx(r)
	defer cancel()

	if _, err := g.bookings.DeleteBooking(ctx, &bookingpb.DeleteBookingRequest{Id: r.PathValue("id")}); err != nil {
		writeGRPCError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
