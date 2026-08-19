<?php
/**
 * Copyright (c) Govard contributors.
 * Distributed under the terms of the repository LICENSE file.
 */

declare(strict_types=1);

namespace Govard\AuditSample\Model;

/**
 * Minimal deterministic model used to exercise the lint toolchain.
 *
 * It references no Magento symbol on purpose: project and module-in-project
 * targets are analyzed straight off the read-only source mount with no Composer
 * install, so any framework reference would resolve to an unknown class and turn
 * a fixture into a source of non-deterministic findings.
 */
class Greeting
{
    /**
     * @var string
     */
    private $subject;

    /**
     * @param string $subject
     */
    public function __construct(string $subject)
    {
        $this->subject = $subject;
    }

    /**
     * Build the greeting message.
     *
     * @return string
     */
    public function message(): string
    {
        return 'Hello, ' . $this->subject . '.';
    }
}
