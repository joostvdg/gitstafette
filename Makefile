.PHONY: docs
docs:
	mkdocs serve

.PHONY: publish-docs
publish-docs: 
	mkdocs gh-deploy -b gh-pages