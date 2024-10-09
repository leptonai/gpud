package query

import "testing"

func TestValidateIbstatOutputErrIbstatOutputBrokenStateDown(t *testing.T) {
	t.Parallel()

	outputWithErr := `

CA 'mlx5_11'
	CA type: MT4129
	Number of ports: 1
	Firmware version: 28.39.1002
	Hardware version: 0
	Node GUID: 0xa088c20300bb3514
	System image GUID: 0xa088c20300bb3514
	Port 1:
		State: Down
		Physical state: Disabled
		Rate: 40
		Base lid: 0
		LMC: 0
		SM lid: 0
		Capability mask: 0x00010000
		Port GUID: 0x0000000000000000
		Link layer: Ethernet
	`
	err := ValidateIbstatOutput(outputWithErr)
	if err != ErrIbstatOutputBrokenStateDown {
		t.Errorf("ibstat output did not pass validation")
	}
}

func TestValidateIbstatOutputErrIbstatOutputBrokenPhysicalDisabled(t *testing.T) {
	t.Parallel()

	outputWithErr := `

CA 'mlx5_11'
	CA type: MT4129
	Number of ports: 1
	Firmware version: 28.39.1002
	Hardware version: 0
	Node GUID: 0xa088c20300bb3514
	System image GUID: 0xa088c20300bb3514
	Port 1:
		State: Active
		Physical state: Disabled
		Rate: 40
		Base lid: 0
		LMC: 0
		SM lid: 0
		Capability mask: 0x00010000
		Port GUID: 0x0000000000000000
		Link layer: Ethernet
	`
	err := ValidateIbstatOutput(outputWithErr)
	if err != ErrIbstatOutputBrokenPhysicalDisabled {
		t.Errorf("ibstat output did not pass validation")
	}
}

func TestValidateIbstatOutputHealthyCase(t *testing.T) {
	t.Parallel()

	outputWithNoErr := `

CA 'mlx5_1'
	CA type: MT4125
	Number of ports: 1
	Firmware version: 22.39.1002
	Hardware version: 0
	Node GUID: 0xb83fd203002a1a1c
	System image GUID: 0xb83fd203002a1a1c
	Port 1:
		State: Active
		Physical state: LinkUp
		Rate: 100
		Base lid: 0
		LMC: 0
		SM lid: 0
		Capability mask: 0x00010000
		Port GUID: 0x0000000000000000
		Link layer: Ethernet

CA 'mlx5_10'
	CA type: MT4129
	Number of ports: 1
	Firmware version: 28.39.1002
	Hardware version: 0
	Node GUID: 0xa088c20300bb98b4
	System image GUID: 0xa088c20300bb98b4
	Port 1:
		State: Active
		Physical state: LinkUp
		Rate: 400
		Base lid: 0
		LMC: 0
		SM lid: 0
		Capability mask: 0x00010000
		Port GUID: 0xa288c2fffebb98b4
		Link layer: Ethernet
	`
	err := ValidateIbstatOutput(outputWithNoErr)
	if err != nil {
		t.Error("healthy ibstat output did not pass the validation")
	}
}
