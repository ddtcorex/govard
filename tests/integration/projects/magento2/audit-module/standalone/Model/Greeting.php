<?php
/**
 * Copyright (c) Govard contributors.
 * Distributed under the terms of the repository LICENSE file.
 */

declare(strict_types=1);

namespace Govard\AuditSample\Model;

/**
 * Minimal deterministic model used to exercise the standalone PHP matrix.
 *
 * It references no Magento symbol on purpose: a standalone module's dependency
 * tree rarely contains the Magento framework, so a framework reference would
 * resolve to an unknown class on every PHP version in the matrix.
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
