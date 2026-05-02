
```bash
# Install dependencies
cd bench
uv sync

# Activate virtual environment
source .venv/bin/activate

# Start Go server (if not running)
cd ../quickcrawl && go build -o quickcrawl-server . && ./quickcrawl-server &

# Run benchmark (downloads dataset once, then caches in bench/dataset_cache/)
API_URL=http://localhost:3000 BENCH_MAX_URLS=1000 uv run python bench.py
```
