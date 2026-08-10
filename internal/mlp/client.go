package mlp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// --- wire types for the OMA MLP v3.5 subset this client speaks: Standard
// Location Immediate Service (slir/slia) and Historic Location Immediate
// Service (hlir/hlia).

type mlpSvcInit struct {
	XMLName xml.Name `xml:"svc_init"`
	Ver     string   `xml:"ver,attr"`
	Hdr     mlpHdr   `xml:"hdr"`
	Slir    *mlpSlir `xml:"slir,omitempty"`
	Hlir    *mlpHlir `xml:"hlir,omitempty"`
}
type mlpHdr struct {
	Ver    string    `xml:"ver,attr"`
	Client mlpClient `xml:"client"`
}
type mlpClient struct {
	ID  string `xml:"id"`
	Pwd string `xml:"pwd"`
}
type mlpMsid struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}
type mlpMsids struct {
	Msid []mlpMsid `xml:"msid"`
}
type mlpSlir struct {
	Ver     string      `xml:"ver,attr"`
	ResType string      `xml:"res_type,attr"`
	Msids   mlpMsids    `xml:"msids"`
	EQoP    *mlpEQoP    `xml:"eqop,omitempty"`
	LocType *mlpLocType `xml:"loc_type,omitempty"`
	Prio    *mlpPrio    `xml:"prio,omitempty"`
}
type mlpEQoP struct {
	RespReq *mlpRespReq `xml:"resp_req,omitempty"`
	HorAcc  *mlpHorAcc  `xml:"hor_acc,omitempty"`
}
type mlpRespReq struct {
	Type string `xml:"type,attr"`
}
type mlpHorAcc struct {
	Value string `xml:",chardata"`
}
type mlpLocType struct {
	Type string `xml:"type,attr"`
}
type mlpPrio struct {
	Type string `xml:"type,attr"`
}

type mlpSvcResult struct {
	XMLName xml.Name `xml:"svc_result"`
	Slia    *mlpSlia `xml:"slia"`
	Hlia    *mlpHlia `xml:"hlia"`
}

// mlpHlir is the Historic Location Immediate Request (Sec 5.2.3.8.1):
// a single msid, start_time (required), stop_time (optional -- omitted
// means "up to now").
type mlpHlir struct {
	Ver       string  `xml:"ver,attr"`
	Msid      mlpMsid `xml:"msid"`
	StartTime string  `xml:"start_time"`
	StopTime  string  `xml:"stop_time,omitempty"`
}

// mlpHlia is the Historic Location Immediate Answer (Sec 5.2.3.8.2):
// multiple pos entries mean multiple points in time for the one queried
// target, not multiple targets.
type mlpHlia struct {
	Pos     []mlpPos   `xml:"pos"`
	Result  *mlpResult `xml:"result,omitempty"`
	AddInfo string     `xml:"add_info,omitempty"`
}
type mlpSlia struct {
	Pos     []mlpPos   `xml:"pos"`
	Result  *mlpResult `xml:"result,omitempty"`
	AddInfo string     `xml:"add_info,omitempty"`
}
type mlpPos struct {
	Msid   mlpMsid    `xml:"msid"`
	Pd     *mlpPd     `xml:"pd,omitempty"`
	PosErr *mlpPosErr `xml:"poserr,omitempty"`
}
type mlpPd struct {
	Time  mlpTimeElem `xml:"time"`
	Shape mlpShape    `xml:"shape"`
}
type mlpTimeElem struct {
	UtcOff string `xml:"utc_off,attr"`
	Value  string `xml:",chardata"`
}
type mlpShape struct {
	CircularArea *mlpCircularArea `xml:"CircularArea,omitempty"`
}
type mlpCircularArea struct {
	Coord  mlpCoord `xml:"coord"`
	Radius string   `xml:"radius"`
}
type mlpCoord struct {
	X string `xml:"X"`
	Y string `xml:"Y"`
}
type mlpPosErr struct {
	Result mlpResult   `xml:"result"`
	Time   mlpTimeElem `xml:"time"`
}
type mlpResult struct {
	ResID string `xml:"resid,attr"`
	Value string `xml:",chardata"`
}

// mlpGem is a standalone root document (not wrapped in svc_result) sent
// as an HTTP 404 for any request the GMLC's MLP listener doesn't
// recognize.
type mlpGem struct {
	XMLName xml.Name  `xml:"gem"`
	Result  mlpResult `xml:"result"`
	AddInfo string    `xml:"add_info,omitempty"`
}

const mlpVersion = "3.5.0"

var dmshPattern = regexp.MustCompile(`^(\d+) (\d+) (\d+(?:\.\d+)?)([NSEW])$`)

// decodeDMSH parses MLP's DMSH (degree-minute-second-hemisphere)
// coordinate format to decimal degrees.
func decodeDMSH(s string) (float64, error) {
	m := dmshPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("mlp: invalid DMSH coordinate %q", s)
	}
	deg, _ := strconv.ParseFloat(m[1], 64)
	min, _ := strconv.ParseFloat(m[2], 64)
	sec, _ := strconv.ParseFloat(m[3], 64)
	v := deg + min/60 + sec/3600
	if m[4] == "S" || m[4] == "W" {
		v = -v
	}
	return v, nil
}

// parseMLPTime parses MLP's compact time format ("20020623134453"),
// treated as UTC.
func parseMLPTime(s string) (time.Time, error) {
	return time.ParseInLocation("20060102150405", strings.TrimSpace(s), time.UTC)
}

// formatMLPTime renders a timestamp in MLP's compact time format
// (Sec 5.3.133), e.g. "20020623134453".
func formatMLPTime(t time.Time) string {
	return t.UTC().Format("20060102150405")
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Client speaks OMA MLP directly to a GMLC's MLP listener over HTTP.
//
// MLP's Standard Location Immediate service has no cancellation message
// on the wire -- slir/slia is synchronous request/response, so once
// Submit returns, the request has already reached a terminal state.
type Client struct {
	baseURL    string
	clientID   string
	password   string
	httpClient *http.Client
}

// NewClient returns a Client speaking OMA MLP v3.5 to a GMLC's MLP
// listener at baseURL, authenticating with clientID/password.
func NewClient(baseURL, clientID, password string, timeout time.Duration) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		clientID:   clientID,
		password:   password,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// residStatus maps MLP Sec 5.4 resid values to a client-facing
// StatusError -- used only for a whole-request-level slia <result> (an
// outright rejection, nothing dispatched at all). A per-target <poserr>
// is not an error at all; it's a normal "failed" RequestStatus.
var residStatus = map[string]string{
	"3":   "unauthorized",
	"4":   "unknown_subscriber",
	"6":   "position_method_failure",
	"7":   "timeout",
	"105": "invalid_request",
	"108": "service_not_supported",
	"110": "invalid_request",
	"113": "invalid_request",
}

func statusErrorForResult(r mlpResult) *StatusError {
	code, ok := residStatus[r.ResID]
	if !ok {
		code = "mlp_error"
	}
	return &StatusError{Code: code, Detail: fmt.Sprintf("resid %s: %s", r.ResID, r.Value)}
}

func targetToMsid(t Target) (mlpMsid, error) {
	if t.IMSI != "" {
		return mlpMsid{Type: "IMSI", Value: t.IMSI}, nil
	}
	if t.MSISDN != "" {
		return mlpMsid{Type: "MSISDN", Value: t.MSISDN}, nil
	}
	return mlpMsid{}, fmt.Errorf("target requires IMSI or MSISDN")
}

// buildSlir renders a slir wire request. qos.class and vertical accuracy
// have no MLP eqop equivalent and are not sent -- a genuine protocol
// gap, not an oversight.
func buildSlir(target Target, locType LocationType, highPriority bool, qos *QoS) (mlpSlir, error) {
	m, err := targetToMsid(target)
	if err != nil {
		return mlpSlir{}, err
	}
	s := mlpSlir{Ver: mlpVersion, ResType: "SYNC", Msids: mlpMsids{Msid: []mlpMsid{m}}}
	switch locType {
	case LocationTypeCurrent:
	case LocationTypeCurrentOrLastKnown:
		s.LocType = &mlpLocType{Type: "CURRENT_OR_LAST"}
	default:
		return mlpSlir{}, fmt.Errorf("unsupported location type %q over MLP", locType)
	}
	if highPriority {
		s.Prio = &mlpPrio{Type: "HIGH"}
	}
	if qos != nil {
		var e mlpEQoP
		var any bool
		if qos.HorizontalAccuracyMeters != nil {
			e.HorAcc = &mlpHorAcc{Value: strconv.FormatFloat(*qos.HorizontalAccuracyMeters, 'g', -1, 64)}
			any = true
		}
		switch qos.ResponseTime {
		case "":
		case "low_delay":
			e.RespReq = &mlpRespReq{Type: "LOW_DELAY"}
			any = true
		case "delay_tolerant":
			e.RespReq = &mlpRespReq{Type: "DELAY_TOL"}
			any = true
		}
		if any {
			s.EQoP = &e
		}
	}
	return s, nil
}

func (c *Client) post(ctx context.Context, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/", bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/xml")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("mlp request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read mlp response: %w", err)
	}
	return resp.StatusCode, respBody, nil
}

// buildFailedStatus renders a per-target poserr as a normal "failed"
// RequestStatus, not a Submit error -- the GMLC dispatched the request,
// it just didn't resolve to a position.
func buildFailedStatus(id string, locType LocationType, highPriority bool, posErr *mlpPosErr) *RequestStatus {
	return &RequestStatus{
		ID: id, State: "failed",
		FailureCode:  fmt.Sprintf("%s: %s", posErr.Result.ResID, posErr.Result.Value),
		LocationType: locType, HighPriority: highPriority,
	}
}

// Submit sends a Standard Location Immediate Request (slir) for target
// and blocks for the synchronous slia answer.
func (c *Client) Submit(ctx context.Context, target Target, locType LocationType, highPriority bool, qos *QoS) (*RequestStatus, error) {
	slir, err := buildSlir(target, locType, highPriority, qos)
	if err != nil {
		return nil, &StatusError{Code: "invalid_request", Detail: err.Error()}
	}
	envelope := mlpSvcInit{Ver: mlpVersion, Hdr: mlpHdr{Ver: mlpVersion, Client: mlpClient{ID: c.clientID, Pwd: c.password}}, Slir: &slir}
	body, err := xml.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode slir: %w", err)
	}

	status, respBody, err := c.post(ctx, append([]byte(xml.Header), body...))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		var g mlpGem
		if xml.Unmarshal(respBody, &g) == nil && g.Result.ResID != "" {
			return nil, statusErrorForResult(g.Result)
		}
		return nil, &StatusError{Code: "mlp_error", Detail: "GMLC returned an unrecognized error response"}
	}

	var v mlpSvcResult
	if err := xml.Unmarshal(respBody, &v); err != nil {
		return nil, fmt.Errorf("decode svc_result: %w", err)
	}
	if v.Slia == nil {
		return nil, &StatusError{Code: "mlp_error", Detail: "GMLC response had no slia"}
	}
	if v.Slia.Result != nil {
		return nil, statusErrorForResult(*v.Slia.Result)
	}
	if len(v.Slia.Pos) != 1 {
		return nil, &StatusError{Code: "mlp_error", Detail: fmt.Sprintf("expected exactly 1 pos, got %d", len(v.Slia.Pos))}
	}
	pos := v.Slia.Pos[0]
	id := newRequestID()

	if pos.PosErr != nil {
		return buildFailedStatus(id, locType, highPriority, pos.PosErr), nil
	}
	if pos.Pd == nil || pos.Pd.Shape.CircularArea == nil {
		return nil, &StatusError{Code: "mlp_error", Detail: "slia pos had neither pd nor poserr"}
	}
	ca := pos.Pd.Shape.CircularArea
	lat, err := decodeDMSH(ca.Coord.X)
	if err != nil {
		return nil, fmt.Errorf("decode latitude: %w", err)
	}
	lon, err := decodeDMSH(ca.Coord.Y)
	if err != nil {
		return nil, fmt.Errorf("decode longitude: %w", err)
	}
	radius, err := strconv.ParseFloat(ca.Radius, 64)
	if err != nil {
		return nil, fmt.Errorf("decode radius: %w", err)
	}
	fixTime := time.Now().UTC()
	if t, err := parseMLPTime(pos.Pd.Time.Value); err == nil {
		fixTime = t
	}
	return &RequestStatus{
		ID: id, State: "completed",
		LocationType: locType, HighPriority: highPriority,
		Result: &Result{Time: fixTime, Latitude: lat, Longitude: lon, UncertaintyMeters: radius},
	}, nil
}

// Ready does a best-effort transport-level reachability check: POST a
// request the GMLC's MLP listener won't recognize as any defined service
// and confirm a well-formed HTTP response comes back at all. MLP's HTTP
// binding has no dedicated health endpoint, so this can only prove the
// listener itself is up, not that the GMLC's backend is healthy.
func (c *Client) Ready(ctx context.Context) error {
	envelope := mlpSvcInit{Ver: mlpVersion, Hdr: mlpHdr{Ver: mlpVersion, Client: mlpClient{ID: c.clientID, Pwd: c.password}}}
	body, err := xml.Marshal(envelope)
	if err != nil {
		return err
	}
	_, _, err = c.post(ctx, append([]byte(xml.Header), body...))
	return err
}

// History queries target's recorded fixes via the GMLC's Historic
// Location Immediate service (hlir/hlia), oldest first. A zero stop
// means "up to now".
func (c *Client) History(ctx context.Context, target Target, start, stop time.Time) ([]HistoryPoint, error) {
	m, err := targetToMsid(target)
	if err != nil {
		return nil, &StatusError{Code: "invalid_request", Detail: err.Error()}
	}
	h := &mlpHlir{Ver: mlpVersion, Msid: m, StartTime: formatMLPTime(start)}
	if !stop.IsZero() {
		h.StopTime = formatMLPTime(stop)
	}
	envelope := mlpSvcInit{Ver: mlpVersion, Hdr: mlpHdr{Ver: mlpVersion, Client: mlpClient{ID: c.clientID, Pwd: c.password}}, Hlir: h}
	body, err := xml.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode hlir: %w", err)
	}

	status, respBody, err := c.post(ctx, append([]byte(xml.Header), body...))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		var g mlpGem
		if xml.Unmarshal(respBody, &g) == nil && g.Result.ResID != "" {
			return nil, statusErrorForResult(g.Result)
		}
		return nil, &StatusError{Code: "mlp_error", Detail: "GMLC returned an unrecognized error response"}
	}

	var v mlpSvcResult
	if err := xml.Unmarshal(respBody, &v); err != nil {
		return nil, fmt.Errorf("decode svc_result: %w", err)
	}
	if v.Hlia == nil {
		return nil, &StatusError{Code: "mlp_error", Detail: "GMLC response had no hlia"}
	}
	if v.Hlia.Result != nil {
		// resid 6 (POSITION METHOD FAILURE) is what a GMLC sends
		// specifically for "nothing recorded in this window" -- a normal
		// outcome, not an error condition.
		if v.Hlia.Result.ResID == "6" {
			return nil, nil
		}
		return nil, statusErrorForResult(*v.Hlia.Result)
	}

	points := make([]HistoryPoint, 0, len(v.Hlia.Pos))
	for _, p := range v.Hlia.Pos {
		if p.Pd == nil || p.Pd.Shape.CircularArea == nil {
			continue // a per-point poserr within pos+ -- nothing to show for it
		}
		ca := p.Pd.Shape.CircularArea
		lat, err := decodeDMSH(ca.Coord.X)
		if err != nil {
			continue
		}
		lon, err := decodeDMSH(ca.Coord.Y)
		if err != nil {
			continue
		}
		recordedAt, err := parseMLPTime(p.Pd.Time.Value)
		if err != nil {
			continue
		}
		point := HistoryPoint{RecordedAt: recordedAt, Latitude: lat, Longitude: lon}
		if radius, err := strconv.ParseFloat(ca.Radius, 64); err == nil {
			point.UncertaintyMeters = radius
		}
		points = append(points, point)
	}
	return points, nil
}
