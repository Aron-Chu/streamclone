import uvicorn

from .config import load_config


def main() -> None:
    cfg = load_config()
    uvicorn.run(
        "liveclipper.app:create_app",
        host=cfg.host,
        port=cfg.port,
        factory=True,
        reload=False,
    )


if __name__ == "__main__":
    main()
