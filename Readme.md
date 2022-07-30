# Hardware Topology Assistant


## CANU notes
```
alias canu='docker run -it --rm -e HOME="/home/canu/" -v $(pwd)/:/files:rw artifactory.algol60.net/csm-docker/stable/cray-canu:1.6.9 /usr/local/bin/canu'
```

Bug
```
canu /files : env | grep HOME
HOME=/home/jenkins
canu /files : whoami
canu
canu /files : canu
Cannot create cache directory: /home/jenkins/.canu
```

10G_25G_40G_100G I37,T107
Mountain-TDS-Management K15,U36
HMN J20,U36

Arista J1,T7 - Don't need as the info is in 10G_25G_40G_100G
NMN J15,T16 - Ignoring, as it doesn't provide information

canu validate shcd -a full --shcd "HPE System Hela CCD.revA37.xlsx" \
    --tabs "Mountain-TDS-Management,10G_25G_40G_100G,HMN" \
    --corners K15,U36,I37,T107,J20,U36 \
    --json --out hela-ccj.json

-- Add 4 computes to existing cabinet

canu validate shcd -a full --shcd "HPE System Hela CCD.revA37-test.xlsx" \
    --tabs "Mountain-TDS-Management,10G_25G_40G_100G,HMN,NMN" \
    --corners K15,U36,I37,T107,J20,U41,J15,T20 \
    --json --out hela-ccj-add-river-computes.json


-- Add cabinet

canu validate shcd -a full --shcd "HPE System Hela CCD - Add acbinet.xlsx" \
    --tabs "Mountain-TDS-Management,10G_25G_40G_100G,HMN,NMN" \
    --corners K15,U36,I37,T129,J20,U50,J15,T24 \
    --json --out hela-ccj-add-river-cabinet.json

go run ./cmd/generate-sls-hardware hela-ccj-add-river-cabinet.json cabinet_lookup.yaml application_node_config.yaml

go run ./cmd/diff-sls sls_state_hela.json sls_state.json