#!/bin/python3.11

import logging

from modules.config import Config
from oci.resource_search import ResourceSearchClient
from oci.resource_search.models import StructuredSearchDetails

class Search:

    search_types = [
    "instance",
    "volume",
    "vcn",
    "analyticsinstance",
    "apigateway",
    "bastion",
    "bootvolume",
    "certificateauthority",
    "instancepool",
    "clusterscluster",
    "clustersvirtualnode",
    "containerinstance",
    "datacatalog",
    "application",
    "disworkspace",
    "datascienceproject",
    "autonomousdatabase",
    "cloudexadatainfrastructure",
    "dbsystem",
    "devopsproject",
    "filesystem",
    "functionsfunction",
    "integrationinstance",
    "loadbalancer",
    "drg",
    "networkfirewall",
    "bucket",
    "stream",
    "vault",
    "vbsinstance"
    ]

    def __init__(self, config: Config):
        self.log = logging.getLogger(f'{__name__}.Search')
        self.log.debug('Initializing Search Object')
        self.client = ResourceSearchClient(config.config, signer=config.signer)
        self.log.debug(f'Search client: {self.client}')

        # Create structured search with parameters
        split = ", "
        query = f"query {split.join(self.search_types)} resources where (definedTags.namespace = '{config.args.tag}')"
        query += f" && compartmentId  = '{config.args.compartment}'" if config.args.compartment else ""
        query += f" && compartmentId  != '{config.args.exclude}'" if config.args.exclude else ""

        details = StructuredSearchDetails(query=query)