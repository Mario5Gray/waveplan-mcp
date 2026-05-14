import asyncio, json, os
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client
from mcp.types import TextContent

async def main():
    env = os.environ.copy()
    env["WAVEPLAN_PLAN"] = os.path.expanduser("~/.local/share/waveplan/plans/2026-04-25-txt2art-amiga-execution-waves.json")
    params = StdioServerParameters(command='./waveplan-mcp', env=env)
    async with stdio_client(params) as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()
            result = await session.call_tool("waveplan_version", {})
            content = result.content[0]
            if isinstance(content, TextContent):
                print(content.text)
            else:
                print(content)
asyncio.run(main())