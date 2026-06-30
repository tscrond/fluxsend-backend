def define_env(env):
    @env.macro
    def generate_toc(nav):
        def _walk(items):
            lines = []
            for item in items:
                title = getattr(item, "title", "") or ""
                children = getattr(item, "children", []) or []
                url = getattr(item, "url", None)
                is_section = getattr(item, "is_section", False)

                if children and (is_section or not url):
                    children_html = _walk(children)
                    lines.append(f"<li><strong>{title}</strong>\n<ul>\n{children_html}\n</ul>\n</li>")
                elif url:
                    lines.append(f'<li><a href="{url}">{title}</a></li>')
                elif title:
                    lines.append(f"<li>{title}</li>")
            return "\n".join(lines)

        if hasattr(nav, "items"):
            root = _walk(nav.items)
        elif hasattr(nav, "children"):
            root = _walk(nav.children)
        else:
            root = _walk(list(nav))
        return f"<ul>\n{root}\n</ul>"
