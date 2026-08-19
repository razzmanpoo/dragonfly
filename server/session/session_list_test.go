package session

import (
	"encoding/json"
	"testing"

	"github.com/df-mc/dragonfly/server/player/skin"
)

func TestSkinToProtocolDefaultGeometry(t *testing.T) {
	got := skinToProtocol(skin.Skin{})

	if got.SkinImageWidth != 64 || got.SkinImageHeight != 32 {
		t.Fatalf("default skin dimensions = %dx%d, want 64x32", got.SkinImageWidth, got.SkinImageHeight)
	}
	if len(got.SkinData) != 64*32*4 {
		t.Fatalf("default skin data length = %d, want %d", len(got.SkinData), 64*32*4)
	}
	if len(got.SkinGeometry) == 0 {
		t.Fatal("default skin geometry is empty")
	}

	var geometry struct {
		Geometry []struct {
			Description struct {
				Identifier string `json:"identifier"`
			} `json:"description"`
		} `json:"minecraft:geometry"`
	}
	if err := json.Unmarshal(got.SkinGeometry, &geometry); err != nil {
		t.Fatalf("default skin geometry is invalid JSON: %v", err)
	}
	if len(geometry.Geometry) < 2 || geometry.Geometry[1].Description.Identifier != "geometry.humanoid.custom" {
		t.Fatalf("default skin geometry does not contain classic player geometry")
	}

	modelConfig, err := skin.DecodeModelConfig(got.SkinResourcePatch)
	if err != nil {
		t.Fatalf("default skin resource patch is invalid: %v", err)
	}
	if modelConfig.Default != "geometry.humanoid.custom" {
		t.Fatalf("default geometry selector = %q, want geometry.humanoid.custom", modelConfig.Default)
	}
}

func TestSkinToProtocolPreservesCustomGeometry(t *testing.T) {
	wantModel := []byte(`{"format_version":"1.12.0"}`)
	wantConfig := skin.ModelConfig{Default: "custom.npc"}
	got := skinToProtocol(skin.Skin{
		Model:       wantModel,
		ModelConfig: wantConfig,
	})

	if string(got.SkinGeometry) != string(wantModel) {
		t.Fatalf("custom geometry was replaced")
	}
	decoded, err := skin.DecodeModelConfig(got.SkinResourcePatch)
	if err != nil {
		t.Fatalf("custom resource patch is invalid: %v", err)
	}
	if decoded.Default != wantConfig.Default {
		t.Fatalf("custom geometry selector = %q, want %q", decoded.Default, wantConfig.Default)
	}
}
