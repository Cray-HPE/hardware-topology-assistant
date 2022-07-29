package ccj

// The following structures where taken from CSI (and slight renamed)
// https://github.com/Cray-HPE/cray-site-init/blob/main/pkg/shcd/shcd.go

type Paddle struct {
	Architecture string         `json:"architecture"`
	CanuVersion  string         `json:"canu_version"`
	ShcdFile     string         `json:"shcd_file"`
	UpdatedAt    string         `json:"updated_at"`
	Topology     []TopologyNode `json:"topology"`
}

func (p *Paddle) FindCommonName(commonName string) (TopologyNode, bool) {
	// TODO can a common name be repeated, or is it an unique key?
	for _, tn := range p.Topology {
		if tn.CommonName == commonName {
			return tn, true
		}
	}

	return TopologyNode{}, false
}

func (p *Paddle) FindNodeByID(id int) (TopologyNode, bool) {
	for _, tn := range p.Topology {
		if tn.ID == id {
			return tn, true
		}
	}

	return TopologyNode{}, false
}

type TopologyNode struct {
	Architecture string   `json:"architecture"`
	CommonName   string   `json:"common_name"`
	ID           int      `json:"id"`
	Location     Location `json:"location"`
	Model        string   `json:"model"`
	Ports        []Port   `json:"ports"`
	Type         string   `json:"type"`
	Vendor       string   `json:"vendor"`
}

func (tp *TopologyNode) FindPorts(slot string) []Port {
	// TODO can slot be more than one?
	var ports []Port
	for _, port := range tp.Ports {
		if port.Slot == slot {
			ports = append(ports, port)
		}
	}

	return ports
}

// The Port type defines where things are plugged in
type Port struct {
	DestNodeID int    `json:"destination_node_id"`
	DestPort   int    `json:"destination_port"`
	DestSlot   string `json:"destination_slot"`
	Port       int    `json:"port"`
	Slot       string `json:"slot"`
	Speed      int    `json:"speed"`
}

// The Location type defines where the server physically exists in the datacenter.
type Location struct {
	Elevation   string `json:"elevation"`
	Rack        string `json:"rack"`
	Parent      string `json:"parent"`       // TODO optional field make ptr or add ignore empty
	SubLocation string `json:"sub_location"` // TODO optional make ptr or add ignore empty
}
