<?php
/**
 * The stub storefront.
 *
 * It exists so the sandbox's HTTP verify has something to answer: a sandbox that
 * advertises `deploy.verify.url` and serves nothing fails the last step of the
 * deploy, and this fixture is deployed by a test that asserts the whole pipeline
 * succeeds. Serving a real status from `pub/` is also what makes the check
 * meaningful — `stack.web_root` below is what puts nginx's document root there.
 */
header('Content-Type: text/plain; charset=utf-8');
echo "magento-stub storefront ok\n";
