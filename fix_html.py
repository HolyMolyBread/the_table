# -*- coding: utf-8 -*-
import re
import os

os.chdir(os.path.dirname(os.path.abspath(__file__)))

with open('index.html', 'r', encoding='utf-8') as f:
    content = f.read()

# Remove inline script block and replace with external script reference
pattern = r'</div>\s*\n\s*\n<script>.*?</script>\s*\n</body>'
replacement = '</div>\n\n  <script src="js/main.js"></script>\n</body>'
new_content = re.sub(pattern, replacement, content, flags=re.DOTALL)

with open('index.html', 'w', encoding='utf-8') as f:
    f.write(new_content)

print('Done')
