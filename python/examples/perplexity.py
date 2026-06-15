import os
import sys
from typing import Optional

import dotenv

dotenv.load_dotenv(os.path.join(os.path.dirname(__file__), ".env"))

from google.adk.agents import Agent
from google.adk.models.lite_llm import LiteLlm
from google.adk.tools import FunctionTool
from google.adk.runners import Runner
from google.adk.sessions import InMemorySessionService
from google.genai import types

from quickcrawl import QuickCrawlClient


def create_web_search_tool():
    """Create a web search tool using QuickCrawl."""

    def web_search(query: str, num_results: int = 10) -> dict:
        """
        Search the web and return comprehensive results with content.

        Args:
            query: The search query to research
            num_results: Number of top results to retrieve (default: 10)

        Returns:
            A dictionary containing search results with markdown content, URLs, and metadata.
        """
        api_url = os.environ.get("QUICK_CRAWL_SERVICE")
        client = QuickCrawlClient(api_url=api_url)

        try:
            search_results = client.search(
                query,
                scrape=True,
                formats=["markdown"],
                render_mode="browser",
            )
        except Exception as e:
            return {"error": f"Search failed: {str(e)}", "results": []}

        results = []
        for item in search_results[:num_results]:
            results.append({
                "url": item.get("url", ""),
                "title": item.get("title", ""),
                "snippet": item.get("description", ""),
                "markdown": item.get("markdown", ""),
                "links": item.get("links", [])[:20] if item.get("links") else [],
            })

        return results

    return FunctionTool(func=web_search)


def create_crawl_tool():
    """Create a website crawl tool using QuickCrawl."""

    def crawl_website(url: str, max_depth: int = 2, max_pages: int = 20) -> dict:
        """
        Crawl a website and extract content from multiple pages.

        Args:
            url: The starting URL to crawl
            max_depth: Maximum link depth (default: 2)
            max_pages: Maximum pages to crawl (default: 20)

        Returns:
            A dictionary containing crawled pages with their content and metadata.
        """
        api_url = os.environ.get("QUICK_CRAWL_SERVICE")
        client = QuickCrawlClient(api_url=api_url)

        try:
            results = client.crawl(url, max_depth=max_depth, max_pages=max_pages)
        except Exception as e:
            return {"error": f"Crawl failed: {str(e)}", "pages": [], "total": 0}

        pages = []
        for page in results:
            pages.append({
                "url": page.get("metadata", {}).get("sourceURL", ""),
                "title": page.get("metadata", {}).get("title", ""),
                "markdown": page.get("markdown", ""),
            })

        return {"pages": pages, "total": len(pages)}

    return FunctionTool(func=crawl_website)


def create_scrape_tool():
    """Create a URL scrape tool using QuickCrawl."""

    def scrape_url(url: str, formats: list = None) -> dict:
        """
        Scrape a single URL and return its content.

        Args:
            url: The URL to scrape
            formats: Output formats to return (default: ["markdown", "links"])

        Returns:
            A dictionary containing the scraped content in requested formats.
        """
        if formats is None:
            formats = ["markdown", "links", "html"]

        api_url = os.environ.get("QUICK_CRAWL_SERVICE")
        client = QuickCrawlClient(api_url=api_url)

        try:
            result = client.scrape(url, formats=formats, render_mode="browser")
        except Exception as e:
            return {
                "url": url,
                "error": f"Scrape failed: {str(e)}",
                "markdown": "",
                "html": "",
                "links": [],
                "metadata": {},
            }

        return {
            "url": url,
            "markdown": result.get("markdown", ""),
            "html": result.get("html", ""),
            "links": result.get("links", []),
            "metadata": result.get("metadata", {}),
        }

    return FunctionTool(func=scrape_url)


def create_research_agent(
    model_name: str = "openai/gpt-4o",
    api_key: Optional[str] = None,
):
    """
    Create a Perplexity-style research agent.

    Args:
        model_name: The model to use (default: openai/gpt-4o)
        api_key: OpenAI API key (falls back to OPENAI_API_KEY env var)

    Returns:
        An ADK Agent configured for web research.
    """
    openai_key = api_key or os.environ.get("OPENAI_API_KEY")
    if not openai_key:
        print("Warning: OPENAI_API_KEY not set. Agent may not function properly.")

    web_search = create_web_search_tool()
    crawl_website = create_crawl_tool()
    scrape_url = create_scrape_tool()

    research_prompt = """You are a research assistant similar to Perplexity. Your goal is to provide comprehensive,
accurate, and well-cited answers to user questions by researching the web.

When a user asks a question:
1. Use the web_search tool to find relevant sources
2. Use scrape_url to get detailed content from the most relevant URLs
3. Use crawl_website if you need to explore a specific website thoroughly
4. Synthesize information from multiple sources to provide a comprehensive answer
5. Always cite your sources with URLs

Be thorough but concise. Prioritize recent and authoritative sources.
Format your final answer with clear sections and inline citations.
"""

    agent = Agent(
        model=LiteLlm(model=model_name, api_key=openai_key),
        name="research_agent",
        description="A web research agent that searches, scrapes, and synthesizes information",
        instruction=research_prompt,
        tools=[web_search, crawl_website, scrape_url],
    )

    return agent


class PerplexityClone:
    """A Perplexity-style AI research assistant using Google ADK and QuickCrawl."""

    def __init__(
        self,
        model: str = "openai/gpt-4o",
        api_key: Optional[str] = None,
        user_id: str = "default_user",
        session_id: str = "default_session",
    ):
        """
        Initialize the Perplexity clone.

        Args:
            model: The LLM model to use (default: openai/gpt-4o)
            api_key: OpenAI API key (falls back to OPENAI_API_KEY env var)
            user_id: Unique identifier for the user
            session_id: Unique identifier for the session
        """
        self.model = model
        self.api_key = api_key or os.environ.get("OPENAI_API_KEY")
        self.user_id = user_id
        self.session_id = session_id

        self.session_service = InMemorySessionService()
        self.agent = create_research_agent(model_name=model, api_key=self.api_key)
        self.runner = Runner(
            agent=self.agent,
            app_name="perplexity_clone",
            session_service=self.session_service,
            auto_create_session=True,
        )

    def ask(self, question: str) -> str:
        """
        Ask a question and get a research-powered answer.

        Args:
            question: The user's question

        Returns:
            The agent's response with citations
        """
        content = types.Content(parts=[types.Part(text=question)], role="user")
        response_stream = self.runner.run(
            user_id=self.user_id,
            session_id=self.session_id,
            new_message=content,
        )

        full_response = ""
        for resp in response_stream:
            if hasattr(resp, "content") and resp.content:
                for part in resp.content.parts:
                    if hasattr(part, "text") and part.text:
                        full_response += part.text

        return full_response

    def chat(self, question: str) -> dict:
        """
        Ask a question and get detailed response with metadata.

        Args:
            question: The user's question

        Returns:
            Dictionary with response text and metadata
        """
        response_text = self.ask(question)

        return {
            "answer": response_text,
            "model": self.model,
            "session_id": self.session_id,
        }


def main():
    """Main entry point for interactive use."""
    import argparse

    parser = argparse.ArgumentParser(description="Perplexity Clone using Google ADK and QuickCrawl")
    parser.add_argument("question", nargs="*", help="The question to research")
    parser.add_argument("--model", default="openai/gpt-4o", help="Model to use (default: openai/gpt-4o)")
    parser.add_argument("--api-key", help="OpenAI API key (falls back to OPENAI_API_KEY env var)")
    parser.add_argument("--interactive", "-i", action="store_true", help="Interactive chat mode")

    args = parser.parse_args()

    if not args.interactive and not args.question:
        parser.print_help()
        print("\nExamples:")
        print("  python perplexity.py 'What is the latest news about AI?'")
        print("  python perplexity.py --interactive")
        return

    api_key = args.api_key or os.environ.get("OPENAI_API_KEY")
    if not api_key:
        print("Error: OpenAI API key required. Set OPENAI_API_KEY env var or use --api-key")
        sys.exit(1)

    client = PerplexityClone(model=args.model, api_key=api_key)

    if args.interactive:
        print("Perplexity Clone (type 'exit' or 'quit' to stop)\n")
        while True:
            try:
                question = input("You: ")
                if question.lower() in ("exit", "quit", "q"):
                    break
                if not question.strip():
                    continue

                print("\nResearching...\n")
                answer = client.ask(question)
                print(f"Answer: {answer}\n")
                print("-" * 80)
            except KeyboardInterrupt:
                break
    else:
        question = " ".join(args.question)
        print("Researching...\n")
        answer = client.ask(question)
        print(f"Answer: {answer}")


if __name__ == "__main__":
    main()