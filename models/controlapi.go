package models

type AuctionRequest struct{}
type AuctionResponse struct{}
type PingRequest struct{}
type PingResponse struct{}
type FindWorkloadRequest struct{}
type FindWorkloadResponse struct{}
type DeployRequest struct{}
type DeployResponse struct{}
type UndeployRequest struct{}
type UndeployResponse struct{}
type InfoRequest struct{}
type InfoResponse struct{}
type LameduckRequest struct {
	NodeId string
	Tag    map[string]string
}
type LameduckResponse struct {
	Success bool
	Msg     string
}
