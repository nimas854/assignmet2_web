package main

import (
	"net/http"

	trailpb "trailbooking/gen/trailpb"
)

// POST /trails
func (g *Gateway) createTrail(w http.ResponseWriter, r *http.Request) {
	req := &trailpb.CreateTrailRequest{}
	if !decodeProto(w, r, req) {
		return
	}
	ctx, cancel := g.ctx(r)
	defer cancel()

	trail, err := g.trails.CreateTrail(ctx, req)
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	w.Header().Set("Location", "/trails/"+trail.GetId())
	writeProto(w, http.StatusCreated, trail)
}

// GET /trails
func (g *Gateway) listTrails(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := g.ctx(r)
	defer cancel()

	res, err := g.trails.ListTrails(ctx, &trailpb.ListTrailsRequest{})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeProto(w, http.StatusOK, res)
}

// GET /trails/{id}
func (g *Gateway) getTrail(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := g.ctx(r)
	defer cancel()

	trail, err := g.trails.GetTrail(ctx, &trailpb.GetTrailRequest{Id: r.PathValue("id")})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeProto(w, http.StatusOK, trail)
}

// PUT /trails/{id}
func (g *Gateway) updateTrail(w http.ResponseWriter, r *http.Request) {
	req := &trailpb.UpdateTrailRequest{}
	if !decodeProto(w, r, req) {
		return
	}
	req.Id = r.PathValue("id") // the id always comes from the URL
	ctx, cancel := g.ctx(r)
	defer cancel()

	trail, err := g.trails.UpdateTrail(ctx, req)
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeProto(w, http.StatusOK, trail)
}

// DELETE /trails/{id}
func (g *Gateway) deleteTrail(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := g.ctx(r)
	defer cancel()

	if _, err := g.trails.DeleteTrail(ctx, &trailpb.DeleteTrailRequest{Id: r.PathValue("id")}); err != nil {
		writeGRPCError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
