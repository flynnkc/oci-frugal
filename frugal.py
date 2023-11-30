#!/bin/python3.11

import logging
import argparse

from modules.config import Config
from modules.search import Search


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('-c', '--compartment', help='Compartment to perform operations on',
                        default=None)
    parser.add_argument('-e', '--exclude', help='Compartment OCID to exclude',
                        default=None)
    parser.add_argument('-d', '--debug', action='store_true', help='Enable debug mode')
    parser.add_argument('-a', '--auth', choices=['instance_principal', 'delegation_token'],
                        help='Authentication method if not config file')
    parser.add_argument('-p', '--profile', help='Profile to use from config file \
                        -- Defaults to DEFAULT', default='DEFAULT')
    parser.add_argument('-f', '--file', help='OCI config file location',
                        default='~/.oci/config')
    parser.add_argument('-t', '--tag', help='Tag namespace containing schedule',
                        default='Schedule')
    parser.add_argument('--config', help='Script config file location', default='./.config.yaml')
    args = parser.parse_args()

    # Set log level, get signer, etc.
    config = Config(args)

    log = logging.getLogger(__name__)

    # Search for relevant resources
    search = Search(config)


if __name__ == '__main__':
    main()