import json

from sc_search.mcp_server.bridge import main


def test_bridge_sc_intents(capsys) -> None:
    code = main(["sc_intents", "{}"])
    assert code == 0
    out = json.loads(capsys.readouterr().out)
    assert out["ok"] is True
    assert "primaries" in out["data"]


def test_bridge_unknown_tool(capsys) -> None:
    code = main(["sc_no_such_tool", "{}"])
    assert code == 1
    out = json.loads(capsys.readouterr().out)
    assert out["ok"] is False
